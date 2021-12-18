package tcc

import (
	"encoding/json"
	"reflect"

	gxnet "github.com/dubbogo/gost/net"
	"github.com/pkg/errors"

	"github.com/opentrx/seata-golang/v2/pkg/apis"
	ctx "github.com/opentrx/seata-golang/v2/pkg/client/base/context"
	"github.com/opentrx/seata-golang/v2/pkg/client/proxy"
	"github.com/opentrx/seata-golang/v2/pkg/client/rm"
	"github.com/opentrx/seata-golang/v2/pkg/util/log"
	"github.com/opentrx/seata-golang/v2/pkg/util/time"
)

var (
	TccActionName = "TccActionName"

	TryMethod     = "Try"
	ConfirmMethod = "Confirm"
	CancelMethod  = "Cancel"

	ActionStartTime = "action-start-time"
	ActionName      = "actionName"
	PrepareMethod   = "sys::prepare"
	CommitMethod    = "sys::commit"
	RollbackMethod  = "sys::rollback"
	HostName        = "host-name"

	TccMethodArguments = "arguments"
	TccMethodResult    = "result"

	businessActionContextType = reflect.TypeOf(&ctx.BusinessActionContext{})
)

type TccService interface {
	Try(ctx *ctx.BusinessActionContext) (bool, error)
	Confirm(ctx *ctx.BusinessActionContext) bool
	Cancel(ctx *ctx.BusinessActionContext) bool
}

type TccProxyService interface {
	GetTccService() TccService
}

func ImplementTCC(v TccProxyService) {
	valueOf := reflect.ValueOf(v)
	log.Debugf("[implement] reflect.TypeOf: %s", valueOf.String())

	//拿到指针指向的对象
	valueOfElem := valueOf.Elem()
	//拿到类型，sample里就是 TCCProxyServiceA，后面就用 TCCProxyServiceA 代替B和C
	typeOf := valueOfElem.Type()

	// check incoming interface, incoming interface's elem must be a struct.
	if typeOf.Kind() != reflect.Struct {
		log.Errorf("%s must be a struct ptr", valueOf.String())
		return
	}
	//拿到要代理的对象指针，也就是这个对象的类需要实现Try、Confirm、Cancel
	proxyService := v.GetTccService()	//sample里就是 ServiceA 的对象指针，ServiceA 实现了 TccProxyService interface的Try、Confirm、Cancel方法
	makeCallProxy := func(methodDesc *proxy.MethodDescriptor, resource *TCCResource) func(in []reflect.Value) []reflect.Value {
		return func(in []reflect.Value) []reflect.Value {	//这里面就是修改后的 Try 方法，只是变成了反射的样式
			businessContextValue := in[0]
			businessActionContext := businessContextValue.Interface().(*ctx.BusinessActionContext)
			rootContext := businessActionContext.RootContext
			businessActionContext.XID = rootContext.GetXID()
			businessActionContext.ActionName = resource.ActionName
			if !rootContext.InGlobalTransaction() {	//不会进这里
				args := make([]interface{}, 0)
				args = append(args, businessActionContext)
				return proxy.Invoke(methodDesc, nil, args)
			}

			returnValues, err := proceed(methodDesc, businessActionContext, resource)
			if err != nil {
				return proxy.ReturnWithError(methodDesc, errors.WithStack(err))
			}
			return returnValues
		}
	}

	//遍历 TCCProxyServiceA 里的成员变量，sample里：
	//type TCCProxyServiceA struct {
	//	*ServiceA
	//
	//	Try func(ctx *context.BusinessActionContext) (bool, error) `TccActionName:"ServiceA"`
	//}
	//这里有两个成员变量，但是ServiceA会跳过，因为不是Func
	numField := valueOfElem.NumField()
	for i := 0; i < numField; i++ {
		t := typeOf.Field(i)
		methodName := t.Name
		f := valueOfElem.Field(i)
		//只有成员变量是函数、非0、可写（内存地址有效且首字母大写暴露出来），并且变量名是 Try 的才处理，其他不处理
		if f.Kind() == reflect.Func && f.IsValid() && f.CanSet() && methodName == TryMethod {
			//如果这个函数变量的入参不是1，且入参的类型不是 ctx.BusinessActionContext，那就报错
			if t.Type.NumIn() != 1 && t.Type.In(0) != businessActionContextType {
				panic("prepare method argument is not BusinessActionContext")
			}

			//那tag的key为TccActionName的值，sample里是ServiceA
			actionName := t.Tag.Get(TccActionName)
			if actionName == "" {
				panic("must tag TccActionName")
			}

			commitMethodDesc := proxy.Register(proxyService, ConfirmMethod)	//拿到函数名为 Confirm 的所有反射信息
			cancelMethodDesc := proxy.Register(proxyService, CancelMethod)	//拿到函数名为 Cancel 的所有反射信息
			tryMethodDesc := proxy.Register(proxyService, methodName)	//拿到函数名为 Try 的所有反射信息

			tccResource := &TCCResource{
				ActionName:         actionName,
				PrepareMethodName:  TryMethod,
				CommitMethodName:   ConfirmMethod,
				CommitMethod:       commitMethodDesc,
				RollbackMethodName: CancelMethod,
				RollbackMethod:     cancelMethodDesc,
			}

			//以 tccResource.ActionName 为key，存 tccResource，到 tccResourceManager.ResourceCache 里
			//用来跟TC通信的 BranchCommunicate 的stream方法，收到 BranchCommit 和 BranchRollback 请求时，
			//从 tccResourceManager.ResourceCache 里找到对应的对象操作对应的 Confirm 和 Cancel 方法
			tccResourceManager.RegisterResource(tccResource)

			// do method proxy here:
			//修改 TCCProxyServiceA 的对象的 Try方法为 makeCallProxy 的返回值
			//这里其实 TCCResource 按照对应关系，也应该有个 PrepareMethod，赋值为 tryMethodDesc，但这里把它从struct里拿出来，单独传参
			f.Set(reflect.MakeFunc(f.Type(), makeCallProxy(tryMethodDesc, tccResource)))
			log.Debugf("set method [%s]", methodName)
		}
	}
}

//resource 里应该包含了这个TCC中RM的服务名和try、confirm、cancel的3个函数名与反射描述，但try的反射描述定义的时候没有放到结构体里，而是单独拿出来作为一个参数 methodDesc
//ctx 就是为了传递XID的
func proceed(methodDesc *proxy.MethodDescriptor, ctx *ctx.BusinessActionContext, resource *TCCResource) ([]reflect.Value, error) {
	var (
		args = make([]interface{}, 0)
	)

	//向TC注册，拿到branchID
	branchID, err := doTccActionLogStore(ctx, resource)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ctx.BranchID = branchID

	args = append(args, ctx)
	returnValues := proxy.Invoke(methodDesc, nil, args)
	errValue := returnValues[len(returnValues)-1]
	if errValue.IsValid() && !errValue.IsNil() {
		//报错，也就是try的时候err非空，就报给TC，说 PhaseOneFailed，TC会修改 branch_table 的 status 字段为 PhaseOneFailed
		err := rm.GetResourceManager().BranchReport(ctx.RootContext, ctx.XID, branchID, apis.TCC, apis.PhaseOneFailed, nil)
		if err != nil {
			log.Errorf("branch report err: %v", err)
		}
	}

	return returnValues, nil
}

func doTccActionLogStore(ctx *ctx.BusinessActionContext, resource *TCCResource) (int64, error) {
	ctx.ActionContext[ActionStartTime] = time.CurrentTimeMillis()
	//都是把字符串名字写进去
	ctx.ActionContext[PrepareMethod] = resource.PrepareMethodName
	ctx.ActionContext[CommitMethod] = resource.CommitMethodName
	ctx.ActionContext[RollbackMethod] = resource.RollbackMethodName
	ctx.ActionContext[ActionName] = ctx.ActionName
	ip, err := gxnet.GetLocalIP()
	if err == nil {
		ctx.ActionContext[HostName] = ip
	} else {
		log.Warn("getLocalIP error")
	}

	applicationContext := make(map[string]interface{})
	applicationContext[TccActionContext] = ctx.ActionContext

	applicationData, err := json.Marshal(applicationContext)
	if err != nil {
		log.Errorf("marshal applicationContext failed:%v", applicationContext)
		return 0, err
	}

	//向TC注册branch，branch_table 插入数据，status 为 apis.Registered
	branchID, err := rm.GetResourceManager().BranchRegister(ctx.RootContext, ctx.XID, resource.GetResourceID(), resource.GetBranchType(), applicationData, "")
	if err != nil {
		log.Errorf("TCC branch Register error, xid: %s", ctx.XID)
		return 0, errors.WithStack(err)
	}
	return branchID, nil
}
