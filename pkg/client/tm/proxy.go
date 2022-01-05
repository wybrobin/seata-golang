package tm

import (
	"context"
	"reflect"

	"github.com/pkg/errors"

	ctx "github.com/opentrx/seata-golang/v2/pkg/client/base/context"
	"github.com/opentrx/seata-golang/v2/pkg/client/base/model"
	"github.com/opentrx/seata-golang/v2/pkg/client/proxy"
	"github.com/opentrx/seata-golang/v2/pkg/util/log"
)

type GlobalTransactionProxyService interface {
	GetProxyService() interface{}
	GetMethodTransactionInfo(methodName string) *model.TransactionInfo
}

var (
	typError = reflect.Zero(reflect.TypeOf((*error)(nil)).Elem()).Type()
)

//传进来的对象要实现接口GlobalTransactionProxyService，
//要告诉当前这个proxy：发起事务对象是哪个（GetProxyService），要运行分布式事务的方法有哪些（GetMethodTransactionInfo）
func Implement(v GlobalTransactionProxyService) {
	valueOf := reflect.ValueOf(v)
	log.Debugf("[implement] reflect.TypeOf: %s", valueOf.String())

	//拿到指针指向的对象
	valueOfElem := valueOf.Elem()
	//拿到类型，sample里就是ProxyService
	typeOf := valueOfElem.Type()

	// check incoming interface, incoming interface's elem must be a struct.
	if typeOf.Kind() != reflect.Struct {
		log.Errorf("%s must be a struct ptr", valueOf.String())
		return
	}
	proxyService := v.GetProxyService() //拿到要代理的对象指针，也就是这个对象的一些方法是要运行分布式事务。
	// sample里就是Svc类型的对象，Svc有一个函数CreateSo(ctx context.Context, rollback bool) error

	//定义一个函数变量makeCallProxy，返回值也是个函数
	makeCallProxy := func(methodDesc *proxy.MethodDescriptor, txInfo *model.TransactionInfo) func(in []reflect.Value) []reflect.Value {
		//直接返回这个匿名函数的返回值
		//也就是说，返回了一个匿名函数，这个匿名函数现在相当于有3个参数，makeCallProxy这个函数变量的两个参数 methodDesc、txInfo 和自己函数本身的参数in
		return func(in []reflect.Value) []reflect.Value {
			var (
				args                     []interface{}
				returnValues             []reflect.Value
				suspendedResourcesHolder *SuspendedResourcesHolder //保存了一个Xid的struct指针
			)

			if txInfo == nil {
				// testing phase, this problem should be resolved
				panic(errors.New("transactionInfo does not exist"))
			}

			inNum := len(in)
			//为什么要+1呢，因为从methodDesc.ArgsNum通过反射拿到的输入参数数，包括对象本身作为第一个参数，这有点像python那样
			if inNum+1 != methodDesc.ArgsNum {
				// testing phase, this problem should be resolved
				panic(errors.New("args does not match"))
			}

			//创建了一个空的RootContext，里面成员变量Context存了一个空的context，context.Background 和 context.TODO都是一样的，是空的context。
			//localMap存了一个map[string]interface{}，也是空的。
			//localMap从后面看，是存一些上下文的字符串对应值的，例如"TX_XID"就存了xID的值
			invCtx := ctx.NewRootContext(context.Background())
			for i := 0; i < inNum; i++ {
				//里面循环遍历in，如果里面有类型为context.Context的，并且非空，则替换掉invCtx里面的成员变量Context
				if in[i].Type().String() == "context.Context" {
					if !in[i].IsNil() { //反射的Value用IsNil来判断是否为空，里面区分了不同类型的不同判断标准
						// the user declared context as method's parameter
						//如果in里的context里有key为"TX_XID"的，那么就把value取出来，放到localMap里，key和value都是string
						invCtx = ctx.NewRootContext(in[i].Interface().(context.Context))
					}
				}
				//把所有的反射值in放到args
				args = append(args, in[i].Interface())
			}
			//上面这个循环，将 in []reflect.Value -> args []interface{}

			//根据invCtx里的xid是否已经存在，创建不同的DefaultGlobalTransaction
			//刚开始都是没有的，所以是一个Status=apis.UnknownGlobalStatus，Role=Launcher，XID为空字符串的 DefaultGlobalTransaction
			tx := GetCurrentOrCreate(invCtx)
			defer func() {
				//将 suspendedResourcesHolder 里保存的Xid写到invCtx里的localMap的key为"TX_XID"里
				//没理解为什么要做这个？？？
				err := tx.Resume(suspendedResourcesHolder, invCtx)
				if err != nil {
					log.Error(err)
				}
			}()

			//这个是sample里配置的事务函数 GetMethodTransactionInfo 返回的 model.TransactionInfo 里调用的信息
			//sample里用的是model.Required
			switch txInfo.Propagation {
			case model.Required:
			case model.RequiresNew:
				//第一次请求到这里，suspendedResourcesHolder里的xid也存的空字符串
				suspendedResourcesHolder, _ = tx.Suspend(true, invCtx)
			case model.NotSupported:
				suspendedResourcesHolder, _ = tx.Suspend(true, invCtx)
				returnValues = proxy.Invoke(methodDesc, invCtx, args)
				return returnValues
			case model.Supports:
				if !invCtx.InGlobalTransaction() {
					returnValues = proxy.Invoke(methodDesc, invCtx, args)
					return returnValues
				}
			case model.Never:
				if invCtx.InGlobalTransaction() {
					return proxy.ReturnWithError(methodDesc, errors.Errorf("Existing transaction found for transaction marked with propagation 'never',xid = %s", invCtx.GetXID()))
				}
				returnValues = proxy.Invoke(methodDesc, invCtx, args)
				return returnValues
			case model.Mandatory:
				if !invCtx.InGlobalTransaction() {
					return proxy.ReturnWithError(methodDesc, errors.New("No existing transaction found for transaction marked with propagation 'mandatory'"))
				}
			default:
				return proxy.ReturnWithError(methodDesc, errors.Errorf("Not Supported Propagation: %s", txInfo.Propagation.String()))
			}

			//从tc获得xid，然后把xid放到invCtx这个context里
			beginErr := tx.BeginWithTimeoutAndName(txInfo.TimeOut, txInfo.Name, invCtx)
			if beginErr != nil {
				return proxy.ReturnWithError(methodDesc, errors.WithStack(beginErr))
			}

			//真正调用用户定义的函数
			//这里有了用户定义函数的所有入参和出参类型描述methodDesc，上下文invCtx，入参和出参的值args，其中invCtx里有xid，作为第一个参数传递给原函数，用户再通过这个context把xid传递给RM
			returnValues = proxy.Invoke(methodDesc, invCtx, args)

			//取最后一个参数，之前有判断过，最后一个必须是error类型，代码就在下面
			errValue := returnValues[len(returnValues)-1]

			// todo 只要出错就回滚，未来可以优化一下，某些错误才回滚，某些错误的情况下，可以提交
			if errValue.IsValid() && !errValue.IsNil() {
				//向tc发送rollback
				rollbackErr := tx.Rollback(invCtx)
				if rollbackErr != nil {
					return proxy.ReturnWithError(methodDesc, errors.WithStack(rollbackErr))
				}
				return returnValues
			}

			commitErr := tx.Commit(invCtx)
			if commitErr != nil {
				return proxy.ReturnWithError(methodDesc, errors.WithStack(commitErr))
			}

			return returnValues
		}
	}

	//拿到实现接口GlobalTransactionProxyService的对象，遍历struct里的成员变量
	//sample里就是：
	//type ProxyService struct {
	//	*Svc
	//	CreateSo func(ctx context.Context, rollback bool) error
	//}
	//循环会跳过 *Svc，因为这个不是函数，而是一个指针
	numField := valueOfElem.NumField()
	for i := 0; i < numField; i++ {
		t := typeOf.Field(i)
		methodName := t.Name
		f := valueOfElem.Field(i)
		//如果成员变量是个函数，有效，且可以赋值
		if f.Kind() == reflect.Func && f.IsValid() && f.CanSet() {
			//字段CreateSo会进入这里
			//拿返回值的数量，sample里就是1
			outNum := t.Type.NumOut()

			// The latest return type of the method must be error.
			//如果返回值最后一个不是error类型，警告并不予处理
			if returnType := t.Type.Out(outNum - 1); returnType != typError {
				log.Warnf("the latest return type %s of method %q is not error", returnType, t.Name)
				continue
			}

			//构建 ServiceDescriptor 保存到全局变量 serviceDescriptorMap 里，返回后面构建的 MethodDescriptor
			//ServiceDescriptor就是struct的各种reflect描述，MethodDescriptor就是函数的各种reflect描述
			methodDescriptor := proxy.Register(proxyService, methodName)

			// do method proxy here:
			//将这个函数成员变量设置为makeCallProxy返回的那个函数，同时将methodDesc、txInfo给那个函数用
			//这里相当于把 CreateSo 赋值为 func(in []reflect.Value) []reflect.Value
			f.Set(reflect.MakeFunc(f.Type(), makeCallProxy(methodDescriptor, v.GetMethodTransactionInfo(methodName))))
			log.Debugf("set method [%s]", methodName)
		}
	}
}
