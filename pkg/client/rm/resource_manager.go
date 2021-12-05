package rm

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/opentrx/seata-golang/v2/pkg/apis"
	"github.com/opentrx/seata-golang/v2/pkg/client/base/exception"
	"github.com/opentrx/seata-golang/v2/pkg/client/base/model"
	"github.com/opentrx/seata-golang/v2/pkg/util/log"
	"github.com/opentrx/seata-golang/v2/pkg/util/runtime"
	"google.golang.org/grpc/metadata"
)

var defaultResourceManager *ResourceManager

type ResourceManagerOutbound interface {
	// BranchRegister register branch transaction.
	BranchRegister(ctx context.Context, xid string, resourceID string, branchType apis.BranchSession_BranchType,
		applicationData []byte, lockKeys string) (int64, error)

	// BranchReport report branch transaction status.
	BranchReport(ctx context.Context, xid string, branchID int64, branchType apis.BranchSession_BranchType,
		status apis.BranchSession_BranchStatus, applicationData []byte) error

	// LockQuery lock resource by lockKeys.
	LockQuery(ctx context.Context, xid string, resourceID string, branchType apis.BranchSession_BranchType, lockKeys string) (bool, error)
}

type ResourceManagerInterface interface {
	BranchCommit(ctx context.Context, request *apis.BranchCommitRequest) (*apis.BranchCommitResponse, error)

	BranchRollback(ctx context.Context, request *apis.BranchRollbackRequest) (*apis.BranchRollbackResponse, error)

	// RegisterResource Register a Resource to be managed by Resource Manager.
	RegisterResource(resource model.Resource)

	// UnregisterResource Unregister a Resource from the Resource Manager.
	UnregisterResource(resource model.Resource)

	// GetBranchType ...
	GetBranchType() apis.BranchSession_BranchType
}

type ResourceManager struct {
	addressing     string
	rpcClient      apis.ResourceManagerServiceClient
	managers       map[apis.BranchSession_BranchType]ResourceManagerInterface
	branchMessages chan *apis.BranchMessage
}

func InitResourceManager(addressing string, client apis.ResourceManagerServiceClient) {
	defaultResourceManager = &ResourceManager{
		addressing:     addressing,
		rpcClient:      client,
		managers:       make(map[apis.BranchSession_BranchType]ResourceManagerInterface),
		branchMessages: make(chan *apis.BranchMessage),
	}
	runtime.GoWithRecover(func() {
		defaultResourceManager.branchCommunicate()
	}, nil)
}

func RegisterTransactionServiceServer(rm ResourceManagerInterface) {
	defaultResourceManager.managers[rm.GetBranchType()] = rm
}

func GetResourceManager() *ResourceManager {
	return defaultResourceManager
}

//这个跑在单独的gorouting里
func (manager *ResourceManager) branchCommunicate() {
	for {
		ctx := metadata.AppendToOutgoingContext(context.Background(), "addressing", manager.addressing)
		stream, err := manager.rpcClient.BranchCommunicate(ctx)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		done := make(chan bool)
		//又起了一个gorouting
		runtime.GoWithRecover(func() {
			for {
				select {
				case _, ok := <-done:
					if !ok {
						return
					}
				case msg := <-manager.branchMessages:
					err := stream.Send(msg)	//不停地收来自 branchMessages 管道的消息，通过pb协议 BranchCommunicate 的stream发送给tc
					if err != nil {
						return
					}
				}
			}
		}, nil)

		for {
			msg, err := stream.Recv()	//接收来自tc通过协议 BranchCommunicate 的stream来的消息
			if err == io.EOF {	//服务端返回 BranchCommunicate 函数了
				close(done)	//让发送消息的gorouting退出
				break
			}
			if err != nil {
				close(done)
				break
			}
			switch msg.BranchMessageType {
			case apis.TypeBranchCommit:	//这个就是tc让rm commit
				request := &apis.BranchCommitRequest{}
				data := msg.GetMessage().GetValue()
				err := request.Unmarshal(data)	//解码 google.protobuf.Any 的二进制数据为 BranchCommitRequest 结构，所以发送的时候也应该用 BranchCommitRequest 结构编码
				if err != nil {
					log.Error(err)
					continue
				}
				response, err := manager.BranchCommit(context.Background(), request)
				if err == nil {
					content, err := types.MarshalAny(response)
					if err == nil {
						manager.branchMessages <- &apis.BranchMessage{
							ID:                msg.ID,
							BranchMessageType: apis.TypeBranchCommitResult,
							Message:           content,
						}
					}
				}
			case apis.TypeBranchRollback:
				request := &apis.BranchRollbackRequest{}
				data := msg.GetMessage().GetValue()
				err := request.Unmarshal(data)
				if err != nil {
					log.Error(err)
					continue
				}
				response, err := manager.BranchRollback(context.Background(), request)
				if err == nil {
					content, err := types.MarshalAny(response)
					if err == nil {
						manager.branchMessages <- &apis.BranchMessage{
							ID:                msg.ID,
							BranchMessageType: apis.TypeBranchRollBackResult,
							Message:           content,
						}
					}
				}
			}
		}
		err = stream.CloseSend()
		if err != nil {
			log.Error(err)
		}
	}
}

func (manager *ResourceManager) BranchRegister(ctx context.Context, xid string, resourceID string,
	branchType apis.BranchSession_BranchType, applicationData []byte, lockKeys string) (int64, error) {
	request := &apis.BranchRegisterRequest{
		Addressing:      manager.addressing,
		XID:             xid,
		ResourceID:      resourceID,
		LockKey:         lockKeys,
		BranchType:      branchType,
		ApplicationData: applicationData,
	}
	resp, err := manager.rpcClient.BranchRegister(ctx, request)
	if err != nil {
		return 0, err
	}
	if resp.ResultCode == apis.ResultCodeSuccess {
		return resp.BranchID, nil
	}
	return 0, &exception.TransactionException{
		Code:    resp.GetExceptionCode(),
		Message: resp.GetMessage(),
	}
}

func (manager *ResourceManager) BranchReport(ctx context.Context, xid string, branchID int64,
	branchType apis.BranchSession_BranchType, status apis.BranchSession_BranchStatus, applicationData []byte) error {
	request := &apis.BranchReportRequest{
		XID:             xid,
		BranchID:        branchID,
		BranchType:      branchType,
		BranchStatus:    status,
		ApplicationData: applicationData,
	}
	resp, err := manager.rpcClient.BranchReport(ctx, request)
	if err != nil {
		return err
	}
	if resp.ResultCode == apis.ResultCodeFailed {
		return &exception.TransactionException{
			Code:    resp.GetExceptionCode(),
			Message: resp.GetMessage(),
		}
	}
	return nil
}

func (manager *ResourceManager) LockQuery(ctx context.Context, xid string, resourceID string, branchType apis.BranchSession_BranchType,
	lockKeys string) (bool, error) {
	request := &apis.GlobalLockQueryRequest{
		XID:        xid,
		ResourceID: resourceID,
		LockKey:    lockKeys,
		BranchType: branchType,
	}

	resp, err := manager.rpcClient.LockQuery(ctx, request)
	if err != nil {
		return false, err
	}
	if resp.ResultCode == apis.ResultCodeSuccess {
		return resp.Lockable, nil
	}
	return false, &exception.TransactionException{
		Code:    resp.GetExceptionCode(),
		Message: resp.GetMessage(),
	}
}

func (manager ResourceManager) BranchCommit(ctx context.Context, request *apis.BranchCommitRequest) (*apis.BranchCommitResponse, error) {
	rm, ok := manager.managers[request.BranchType]	//BranchType 就是 AT TCC SAGA XA
	if ok {
		return rm.BranchCommit(ctx, request)	//如果是AT，跳转的时候要用sample代码，因为AT写在opentrx的mysql库里，这里看只有TCC的。AT就是删除undo_log表的对应数据
	}
	return &apis.BranchCommitResponse{
		ResultCode: apis.ResultCodeFailed,
		Message:    fmt.Sprintf("there is no resource manager for %s", request.BranchType.String()),
	}, nil
}

func (manager *ResourceManager) BranchRollback(ctx context.Context, request *apis.BranchRollbackRequest) (*apis.BranchRollbackResponse, error) {
	rm, ok := manager.managers[request.BranchType]
	if ok {
		return rm.BranchRollback(ctx, request)	//和 BranchCommit 一样，要到mysql库里看
	}
	return &apis.BranchRollbackResponse{
		ResultCode: apis.ResultCodeFailed,
		Message:    fmt.Sprintf("there is no resource manager for %s", request.BranchType.String()),
	}, nil
}

func (manager *ResourceManager) RegisterResource(resource model.Resource) {
	rm := manager.managers[resource.GetBranchType()]
	rm.RegisterResource(resource)
}

func (manager *ResourceManager) UnregisterResource(resource model.Resource) {
	rm := manager.managers[resource.GetBranchType()]
	rm.UnregisterResource(resource)
}
