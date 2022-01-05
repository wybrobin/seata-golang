package client

import (
	"log"

	"google.golang.org/grpc"

	"github.com/opentrx/seata-golang/v2/pkg/apis"
	"github.com/opentrx/seata-golang/v2/pkg/client/config"
	"github.com/opentrx/seata-golang/v2/pkg/client/rm"
	"github.com/opentrx/seata-golang/v2/pkg/client/tcc"
	"github.com/opentrx/seata-golang/v2/pkg/client/tm"
)

// Init init resource manager，init transaction manager, expose a port to listen tc
// call back request.
func Init(config *config.Configuration) {
	var conn *grpc.ClientConn
	var err error
	if config.GetClientTLS() == nil {
		conn, err = grpc.Dial(config.ServerAddressing,
			grpc.WithInsecure(),
			grpc.WithKeepaliveParams(config.GetClientParameters()))
	} else {
		conn, err = grpc.Dial(config.ServerAddressing,
			grpc.WithKeepaliveParams(config.GetClientParameters()), grpc.WithTransportCredentials(config.GetClientTLS()))
	}

	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	resourceManagerClient := apis.NewResourceManagerServiceClient(conn)
	transactionManagerClient := apis.NewTransactionManagerServiceClient(conn)

	//不管是TM还是RM，都会调用这个，其实连接是一个，只是绑定了两个rpc协议，所以都调用问题不大

	//初始化RM的rpc client：defaultResourceManager，也就是protobuf的 ResourceManagerServiceClient
	//同时向TC发送 BranchCommunicate 请求，创建一个长连接，让TC通知RM对某个xid是commit还是rollback
	rm.InitResourceManager(config.Addressing, resourceManagerClient)
	//初始化TM的rpc client：defaultTransactionManager，也就是protobuf的 TransactionManagerServiceClient
	tm.InitTransactionManager(config.Addressing, transactionManagerClient)
	//将tcc的 tccResourceManager 放到 defaultResourceManager.managers 里
	rm.RegisterTransactionServiceServer(tcc.GetTCCResourceManager())
}
