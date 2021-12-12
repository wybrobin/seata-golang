package proxy

import (
	"context"
	"reflect"
	"sync"
	"unicode"
	"unicode/utf8"

	ctx "github.com/opentrx/seata-golang/v2/pkg/client/base/context"
	"github.com/opentrx/seata-golang/v2/pkg/util/log"
)

var (
	// serviceDescriptorMap, string -> *ServiceDescriptor
	serviceDescriptorMap = sync.Map{}
)

// MethodDescriptor
type MethodDescriptor struct {
	Method           reflect.Method
	CallerValue      reflect.Value
	CtxType          reflect.Type
	ArgsType         []reflect.Type
	ArgsNum          int
	ReturnValuesType []reflect.Type
	ReturnValuesNum  int
}

// ServiceDescriptor
type ServiceDescriptor struct {
	Name         string
	ReflectType  reflect.Type
	ReflectValue reflect.Value
	Methods      sync.Map // string -> *MethodDescriptor
}

// Register
//sample里传进来的就是Svc的对象指针和它与包裹它的ProxyService的函数变量一样的函数名CreateSo
//TCC传进来的是ServiceA 的对象，实现了 TccProxyService interface的Try、Confirm、Cancel方法
//这里就是把这个服务的反射数据都放到以服务名为key的 serviceDescriptorMap 变量里，包括服务名、对象指针的TypeOf和ValueOf，还有所有方法的context参数和其他入参，还有出参
//最后返回 MethodDescriptor 类型，这个也存在了 serviceDescriptorMap 对应服务里的 Methods 里
func Register(service interface{}, methodName string) *MethodDescriptor {
	serviceType := reflect.TypeOf(service)	//*Svc，TCC是 *ServiceA
	serviceValue := reflect.ValueOf(service)	//这个value是个指针
	svcName := reflect.Indirect(serviceValue).Type().Name()	//Indirect就是如果是指针，则返回.Elem()，如果不是指针，则直接返回值，所以这里拿到的是Svc
	//TCC 就是 ServiceA

	//全局变量，以struct的名字为key，ServiceDescriptor指针为value的map，存在则直接取，不存在则新建。
	//ServiceDescriptor保存了这个struct的名字（Name），指针TypeOf（ReflectType）和指针valueOf（ReflectValue），还有一个空的sync.Map（Methods）
	svcDesc, _ := serviceDescriptorMap.LoadOrStore(svcName, &ServiceDescriptor{
		Name:         svcName,
		ReflectType:  serviceType,
		ReflectValue: serviceValue,
		Methods:      sync.Map{},
	})
	svcDescriptor := svcDesc.(*ServiceDescriptor)
	//新建的时候，Methods是空的，所以会跳过下面的if
	methodDesc, methodExist := svcDescriptor.Methods.Load(methodName)
	if methodExist {
		methodDescriptor := methodDesc.(*MethodDescriptor)
		return methodDescriptor
	}

	//通过reflect.Type和函数名，拿到reflect.Method，这里用reflect.Value也能通过MethodByName拿到
	method, methodFounded := serviceType.MethodByName(methodName)
	if methodFounded {
		//构建MethodDescriptor指针
		//type MethodDescriptor struct {
		//	Method           reflect.Method	//函数的reflect.Method
		//	CallerValue      reflect.Value	//构建时没有赋值
		//	CtxType          reflect.Type	//context.Context这个类型的reflect.Type
		//	ArgsType         []reflect.Type	//所有入参类型组成的切片
		//	ArgsNum          int			//入参个数，包括了对象本身，所以会多一个
		//	ReturnValuesType []reflect.Type	//所有出参类型组成的切片
		//	ReturnValuesNum  int			//出参个数
		//}
		methodDescriptor := describeMethod(method)
		if methodDescriptor != nil {
			methodDescriptor.CallerValue = serviceValue	//函数所有者指针，也就是sample里的*Svc
			svcDescriptor.Methods.Store(methodName, methodDescriptor)	//把MethodDescriptor存到ServiceDescriptor里的Methods的Map里
			return methodDescriptor	//ServiceDescriptor 前面构建是为了存到全局变量 serviceDescriptorMap 里，最终就返回后面构建的 MethodDescriptor
		}
	}
	return nil
}

// describeMethod
// might return nil when method is not exported or some other error
func describeMethod(method reflect.Method) *MethodDescriptor {
	methodType := method.Type	//拿到这个method的reflect.Type
	methodName := method.Name	//sample里就是CreateSo
	inNum := methodType.NumIn()	//这里返回3，其实是2个参数，但第一个参数是struct对象本身
	outNum := methodType.NumOut()	//1

	// Method must be exported.
	//这里是防止函数名第一个字母小写，小写就没有导出，但是我发现小写连MethodByName都拿不到，就不会进到这里了？？？
	if method.PkgPath != "" {
		return nil
	}

	var (
		ctxType                    reflect.Type
		argsType, returnValuesType []reflect.Type
	)

	for index := 1; index < inNum; index++ {	//这里也从下标1开始，直接跳过第一个参数struct本身
		if methodType.In(index).String() == "context.Context" {
			ctxType = methodType.In(index)	//就是保存了context.Context这个类型的reflect.Type
		}
		argsType = append(argsType, methodType.In(index))	//把入参的类型追加到argsType后面
		// need not be a pointer.
		//入参类型也是不能私有的
		if !isExportedOrBuiltinType(methodType.In(index)) {
			log.Errorf("argument type of method %q is not exported %v", methodName, methodType.In(index))
			return nil
		}
	}

	// returnValuesType
	//将返回值追加到returnValuesType后面，并且不能是私有的
	for num := 0; num < outNum; num++ {
		returnValuesType = append(returnValuesType, methodType.Out(num))
		// need not be a pointer.
		if !isExportedOrBuiltinType(methodType.Out(num)) {
			log.Errorf("reply type of method %s not exported{%v}", methodName, methodType.Out(num))
			return nil
		}
	}

	return &MethodDescriptor{
		Method:           method,
		CtxType:          ctxType,
		ArgsType:         argsType,
		ArgsNum:          inNum,
		ReturnValuesType: returnValuesType,
		ReturnValuesNum:  outNum,
	}
}

// Is this an exported - upper case - name
func isExported(name string) bool {
	s, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(s)
}

// Is this type exported or a builtin?
func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// PkgPath will be non-empty even for an exported type,
	// so we need to check the type name as well.
	return isExported(t.Name()) || t.PkgPath() == ""
}

// Invoke
func Invoke(methodDesc *MethodDescriptor, ctx *ctx.RootContext, args []interface{}) []reflect.Value {
	//这里因为用TypeOf的方法发射，所以要指名第一个入参是具体的哪个对象的Value
	in := []reflect.Value{methodDesc.CallerValue}

	for i := 0; i < len(args); i++ {
		t := reflect.ValueOf(args[i])
		//如果参数类型是context.Context，那么就判断一下这个值是否有效，如果没有效，可能是个nil，那么就用methodDesc.CtxType类型创建一个空值的context
		if methodDesc.ArgsType[i].String() == "context.Context" {
			t = SuiteContext(ctx, methodDesc)
		}
		//t是否是零值
		if !t.IsValid() {	//是零值，就用methodDesc.ArgsType新建一个对象，这里应该是怕空指针的情况
			at := methodDesc.ArgsType[i]
			if at.Kind() == reflect.Ptr {
				at = at.Elem()
			}
			t = reflect.New(at)
		}
		in = append(in, t)
	}

	//真正的反射函数调用
	returnValues := methodDesc.Method.Func.Call(in)

	return returnValues
}

func SuiteContext(ctx context.Context, methodDesc *MethodDescriptor) reflect.Value {
	if contextValue := reflect.ValueOf(ctx); contextValue.IsValid() {
		return contextValue
	}
	return reflect.Zero(methodDesc.CtxType)
}

func ReturnWithError(methodDesc *MethodDescriptor, err error) []reflect.Value {
	var result = make([]reflect.Value, 0)
	for i := 0; i < methodDesc.ReturnValuesNum-1; i++ {
		result = append(result, reflect.Zero(methodDesc.ReturnValuesType[i]))
	}
	result = append(result, reflect.ValueOf(err))
	return result
}
