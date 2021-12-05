package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/opentrx/seata-golang/v2/pkg/apis"
	common2 "github.com/opentrx/seata-golang/v2/pkg/common"
	"github.com/opentrx/seata-golang/v2/pkg/tc/config"
	"github.com/opentrx/seata-golang/v2/pkg/tc/event"
	"github.com/opentrx/seata-golang/v2/pkg/tc/holder"
	"github.com/opentrx/seata-golang/v2/pkg/tc/lock"
	"github.com/opentrx/seata-golang/v2/pkg/tc/model"
	"github.com/opentrx/seata-golang/v2/pkg/tc/storage/driver/factory"
	"github.com/opentrx/seata-golang/v2/pkg/util/common"
	"github.com/opentrx/seata-golang/v2/pkg/util/log"
	"github.com/opentrx/seata-golang/v2/pkg/util/runtime"
	time2 "github.com/opentrx/seata-golang/v2/pkg/util/time"
	"github.com/opentrx/seata-golang/v2/pkg/util/uuid"
)

const AlwaysRetryBoundary = 0

type TransactionCoordinator struct {
	sync.Mutex
	maxCommitRetryTimeout            int64
	maxRollbackRetryTimeout          int64
	rollbackRetryTimeoutUnlockEnable bool

	asyncCommittingRetryPeriod time.Duration
	committingRetryPeriod      time.Duration
	rollingBackRetryPeriod     time.Duration
	timeoutRetryPeriod         time.Duration

	streamMessageTimeout time.Duration

	holder             *holder.SessionHolder	//感觉这个都是操作global_table和branch_table表的
	resourceDataLocker *lock.LockManager	//感觉这个都是操作lock_table表的
	locker             GlobalSessionLocker

	idGenerator        *atomic.Uint64
	futures            *sync.Map	//这个是当tc要已经通过 callBackMessages 里的value存的chan给RM发送消息后，RM的响应都会去检查 futures 里的数据，塞的数据是 MessageFuture 里面有个 chan Done，
									// tc的 branchCommit 或 branchRollback 会等待这个Done来信号，而RM给TC回消息后，就会向这个Done发送一个true。如果超时等不到， branchCommit 或 branchRollback 会报错。
	activeApplications *sync.Map
	callBackMessages   *sync.Map	//这个放入的value，是发送给RM的 BranchCommunicate 的数据，当TM需要commit或rollback时，是往这个value里存的chan塞数据，
									// 然后 BranchCommunicate 的服务函数会起个gorouting从chan里取数据，然后stream.Send(msg)
}

func NewTransactionCoordinator(conf *config.Configuration) *TransactionCoordinator {
	//根据config.yml里的配置，生成一个storage.Driver对象，这个对象可能是mysql.driver、pgsql.driver或inmemory.driver
	driver, err := factory.Create(conf.Storage.Type(), conf.Storage.Parameters())
	if err != nil {
		log.Fatalf("failed to construct %s driver: %v", conf.Storage.Type(), err)
		os.Exit(1)
	}
	tc := &TransactionCoordinator{
		maxCommitRetryTimeout:            conf.Server.MaxCommitRetryTimeout,
		maxRollbackRetryTimeout:          conf.Server.MaxRollbackRetryTimeout,
		rollbackRetryTimeoutUnlockEnable: conf.Server.RollbackRetryTimeoutUnlockEnable,

		asyncCommittingRetryPeriod: conf.Server.AsyncCommittingRetryPeriod,
		committingRetryPeriod:      conf.Server.CommittingRetryPeriod,
		rollingBackRetryPeriod:     conf.Server.RollingBackRetryPeriod,
		timeoutRetryPeriod:         conf.Server.TimeoutRetryPeriod,

		streamMessageTimeout: conf.Server.StreamMessageTimeout,

		holder:             holder.NewSessionHolder(driver),	//storage.Driver 接口继承了 storage.SessionManager，SessionHolder里包含了一个SessionManager对象
		resourceDataLocker: lock.NewLockManager(driver),		//storage.Driver 接口继承了 storage.LockManager，lock.LockManager里包含了一个storage.LockManager
		locker:             new(UnimplementedGlobalSessionLocker),	//全局session lock 里的TryLock和Unlock是空实现？

		idGenerator:        &atomic.Uint64{},
		futures:            &sync.Map{},
		activeApplications: &sync.Map{},
		callBackMessages:   &sync.Map{},
	}
	//每秒检查一次状态为 apis.Begin 而且超时的，放到管道event.EventBus.GlobalTransactionEventChannel里，然后把状态改为TimeoutRollingBack
	//这样就会交给 go tc.processRetryRollingBack() 处理，然后由这个gorouting进行回滚
	//在metrics，看样子是暴露给promethues一个http接口，用于监控，统计处理了多少个事务
	go tc.processTimeoutCheck()
	//找到global_table表里status是AsyncCommitting的数据，对它们branch进行分析，然后判断是否commit
	//用来处理那些不是TCC，标记为 AsyncCommitting 的数据，在这里异步的commit
	go tc.processAsyncCommitting()
	//处理status为CommitRetrying的global_table和branch_table数据，对它们branch进行分析，然后判断是否将状态改为commit
	//当commit失败时，就由这个gorouting来重试
	go tc.processRetryCommitting()
	//处理status为RollingBack、RollbackRetrying、TimeoutRollingBack、TimeoutRollbackRetrying的global_table和branch_table数据，对它们branch进行分析，然后判断是否将状态改为rollback
	go tc.processRetryRollingBack()

	return tc
}

//tc收到tm的获取xid的请求
func (tc *TransactionCoordinator) Begin(ctx context.Context, request *apis.GlobalBeginRequest) (*apis.GlobalBeginResponse, error) {
	transactionID := uuid.NextID()
	//xid格式：addressing:tranID，这样就是具有不同addressing名字的，也就是不同的tm，但用相同的tc，他们加起来超过每毫秒4096也不会重复，除非单个tm每毫秒超过4096
	xid := common.GenerateXID(request.Addressing, transactionID)
	gt := model.GlobalTransaction{
		GlobalSession: &apis.GlobalSession{
			Addressing:      request.Addressing,	//sample里就是config.yml里配的addressing：aggregationSvc
			XID:             xid,
			TransactionID:   transactionID,	//其实这里transactionID可以从xid里知道，就是:后面的
			TransactionName: request.TransactionName,	//sample里就是CreateSo，也就是TransactionInfo.Name
			Timeout:         request.Timeout,	//TransactionInfo.TimeOut，也是tm传的
		},
	}
	gt.Begin()	//初始化gt里的状态值，其实这里的proto文件，不仅生成了grpc的协议，还生成了xorm，也就是数据库对象的类型
	err := tc.holder.AddGlobalSession(gt.GlobalSession)	//插表global_table
	if err != nil {
		return &apis.GlobalBeginResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.BeginFailed,
			Message:       err.Error(),
		}, nil
	}

	runtime.GoWithRecover(func() {	//发送给一个StandardCounter去记数+1，在metrics，看样子是暴露给promethues一个http接口，用于监控，统计处理了多少个事务
		evt := event.NewGlobalTransactionEvent(gt.TransactionID, event.RoleTC, gt.TransactionName, gt.BeginTime, 0, gt.Status)
		event.EventBus.GlobalTransactionEventChannel <- evt
	}, nil)

	log.Infof("successfully begin global transaction xid = {}", gt.XID)
	return &apis.GlobalBeginResponse{
		ResultCode: apis.ResultCodeSuccess,
		XID:        xid,
	}, nil
}

func (tc *TransactionCoordinator) GetStatus(ctx context.Context, request *apis.GlobalStatusRequest) (*apis.GlobalStatusResponse, error) {
	gs := tc.holder.FindGlobalSession(request.XID)
	if gs != nil {
		return &apis.GlobalStatusResponse{
			ResultCode:   apis.ResultCodeSuccess,
			GlobalStatus: gs.Status,
		}, nil
	}
	return &apis.GlobalStatusResponse{
		ResultCode:   apis.ResultCodeSuccess,
		GlobalStatus: apis.Finished,
	}, nil
}

func (tc *TransactionCoordinator) GlobalReport(ctx context.Context, request *apis.GlobalReportRequest) (*apis.GlobalReportResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GlobalReport not implemented")
}

//请求参数只有一个XID
func (tc *TransactionCoordinator) Commit(ctx context.Context, request *apis.GlobalCommitRequest) (*apis.GlobalCommitResponse, error) {
	gt := tc.holder.FindGlobalTransaction(request.XID)
	if gt == nil {
		return &apis.GlobalCommitResponse{
			ResultCode:   apis.ResultCodeSuccess,
			GlobalStatus: apis.Finished,
		}, nil
	}
	shouldCommit, err := func(gt *model.GlobalTransaction) (bool, error) {
		//没用，必定返回true
		result, err := tc.locker.TryLock(gt.GlobalSession, time.Duration(gt.Timeout)*time.Millisecond)
		if err != nil {
			return false, err
		}
		if result {	//必定进来
			defer tc.locker.Unlock(gt.GlobalSession)
			if gt.Active {
				// Active need persistence
				// Highlight: Firstly, close the session, then no more branch can be registered.
				//将global_table的这个xid的active设置为0
				err = tc.holder.InactiveGlobalSession(gt.GlobalSession)
				if err != nil {
					return false, err
				}
			}
			//删除lock_table相关记录
			tc.resourceDataLocker.ReleaseGlobalSessionLock(gt)
			if gt.Status == apis.Begin {
				//更新global_table表对应xid的status为Committing
				err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.Committing)
				if err != nil {
					return false, err
				}
				return true, nil
			}
			return false, nil
		}
		return false, fmt.Errorf("failed to lock global transaction xid = %s", request.XID)
	}(gt)

	if err != nil {
		return &apis.GlobalCommitResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.FailedLockGlobalTransaction,
			Message:       err.Error(),
			GlobalStatus:  gt.Status,
		}, nil
	}

	if !shouldCommit {	//gt.Status 进来的时候不是Begin
		if gt.Status == apis.AsyncCommitting {
			return &apis.GlobalCommitResponse{
				ResultCode:   apis.ResultCodeSuccess,
				GlobalStatus: apis.Committed,
			}, nil
		}
		return &apis.GlobalCommitResponse{
			ResultCode:   apis.ResultCodeSuccess,
			GlobalStatus: gt.Status,
		}, nil
	}

	if gt.CanBeCommittedAsync() {//奇怪，为什么不是TCC的就直接变成状态AsyncCommitting了？？？不应该继续执行吗？比如AT。难道不是TCC就是异步提交？TCC就相当于到这一步必须要成功？
		//TC在 NewTransactionCoordinator 的时候，有 go tc.processAsyncCommitting()，这里就是用来处理这些状态为 apis.AsyncCommitting 的，
		//会在gorouting里扫描状态为 apis.AsyncCommitting 的，然后调用 tc.doGlobalCommit(transaction, true)
		err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.AsyncCommitting)
		if err != nil {
			return nil, err
		}
		return &apis.GlobalCommitResponse{
			ResultCode:   apis.ResultCodeSuccess,
			GlobalStatus: apis.Committed,
		}, nil
	}

	//TCC必须同步commit，为什么呢？？？
	_, err = tc.doGlobalCommit(gt, false)
	if err != nil {
		return &apis.GlobalCommitResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.UnknownErr,
			Message:       err.Error(),
			GlobalStatus:  gt.Status,
		}, nil
	}
	return &apis.GlobalCommitResponse{
		ResultCode:   apis.ResultCodeSuccess,
		GlobalStatus: apis.Committed,
	}, nil
}

//检查GlobalTransaction里所有branch的状态，决定是否commit，然后删除对应的表数据
//retrying 参数表示当前 doGlobalCommit 是否在 go tc.processAsyncCommitting() 或 go tc.processRetryCommitting() 里调用，
//如果是，那就不用再标记为 apis.CommitRetrying 了，如果不是，要标记状态为 apis.CommitRetrying ，这样可以让上面两个gorouting去重试
func (tc *TransactionCoordinator) doGlobalCommit(gt *model.GlobalTransaction, retrying bool) (bool, error) {
	var err error

	runtime.GoWithRecover(func() {
		//创建一个GlobalTransactionEvent对象，扔到event.EventBus.GlobalTransactionEventChannel管道里
		evt := event.NewGlobalTransactionEvent(gt.TransactionID, event.RoleTC, gt.TransactionName, gt.BeginTime, 0, gt.Status)
		event.EventBus.GlobalTransactionEventChannel <- evt
	}, nil)

	if gt.IsSaga() {//如果branch里有一个branch_type是2，就算saga，然后就报错，saga不支持
		return false, status.Errorf(codes.Unimplemented, "method Commit not supported saga mode")
	}

	for bs := range gt.BranchSessions {
		//如果状态为PhaseOneFailed，则从branch_table表中删除
		if bs.Status == apis.PhaseOneFailed {
			tc.resourceDataLocker.ReleaseLock(bs)
			delete(gt.BranchSessions, bs)
			//删除branch_table表中xid和branch_id与bs里保存相等的
			err = tc.holder.RemoveBranchSession(gt.GlobalSession, bs)
			if err != nil {
				return false, err
			}
			continue
		}
		branchStatus, err1 := tc.branchCommit(bs)
		if err1 != nil {
			log.Errorf("exception committing branch xid=%d branchID=%d, err: %v", bs.GetXID(), bs.BranchID, err1)
			if !retrying {
				//改global_table表xid对应的状态为CommitRetrying
				err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.CommitRetrying)
				if err != nil {
					return false, err
				}
			}
			return false, err1
		}
		switch branchStatus {
		case apis.PhaseTwoCommitted:
			//如果RM成功删除了undo_log，就会将这个状态返回
			//删除bs对应的branch_table的一条记录下对应所有lock_table表记录
			tc.resourceDataLocker.ReleaseLock(bs)
			delete(gt.BranchSessions, bs)
			//删除branch_table表中bs记录，gt.GlobalSession参数只有inmemory的时候有用
			err = tc.holder.RemoveBranchSession(gt.GlobalSession, bs)
			if err != nil {
				return false, err
			}
			continue
		case apis.PhaseTwoCommitFailedCanNotRetry:
			{
				if gt.CanBeCommittedAsync() {	//不是TCC
					log.Errorf("by [%s], failed to commit branch %v", bs.Status.String(), bs)
					continue
				} else {//是TCC
					// change status first, if need retention global session data,
					// might not remove global session, then, the status is very important.
					err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.CommitFailed)
					if err != nil {
						return false, err
					}
					tc.resourceDataLocker.ReleaseGlobalSessionLock(gt)
					err = tc.holder.RemoveGlobalTransaction(gt)
					if err != nil {
						return false, err
					}
					log.Errorf("finally, failed to commit global[%d] since branch[%d] commit failed", gt.XID, bs.BranchID)
					return false, nil
				}
			}
		default:
			{
				if !retrying {
					err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.CommitRetrying)
					if err != nil {
						return false, err
					}
					return false, nil
				}
				if gt.CanBeCommittedAsync() {
					log.Errorf("by [%s], failed to commit branch %v", bs.Status.String(), bs)
					continue
				} else {
					log.Errorf("failed to commit global[%d] since branch[%d] commit failed, will retry later.", gt.XID, bs.BranchID)
					return false, nil
				}
			}
		}
	}

	//能到这里的bs是status==PhaseOneFailed 或 PhaseTwoCommitted 或 PhaseTwoCommitFailedCanNotRetry且不是TCC 或 其他状态且不是TCC
	//我理解就是已经完事了的，要么彻底失败，要么彻底成功的

	//这里只是检查一下是否有branch？？？为啥不用gt直接检查，要生成一个gs？
	gs := tc.holder.FindGlobalTransaction(gt.XID)
	if gs != nil && gs.HasBranch() {
		log.Infof("global[%d] committing is NOT done.", gt.XID)
		return false, nil
	}

	// change status first, if need retention global session data,
	// might not remove global session, then, the status is very important.
	//没看到这行注释什么意思，已经要删了，为啥还要修改状态？？？
	err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.Committed)
	if err != nil {
		return false, err
	}

	//删除gt的lock_table相关所有内容
	tc.resourceDataLocker.ReleaseGlobalSessionLock(gt)
	//删除gt的global_table和branch_table相关所有内容，
	//这里可能存在 PhaseTwoCommitFailedCanNotRetry且不是TCC 这种，所以还要再删一次branch_table相关所有内容
	//像 PhaseOneFailed 或 PhaseTwoCommitted 在上面已经删了branch_table相关内容
	err = tc.holder.RemoveGlobalTransaction(gt)
	if err != nil {
		return false, err
	}
	runtime.GoWithRecover(func() {
		evt := event.NewGlobalTransactionEvent(gt.TransactionID, event.RoleTC, gt.TransactionName, gt.BeginTime,
			int64(time2.CurrentTimeMillis()), gt.Status)
		event.EventBus.GlobalTransactionEventChannel <- evt
	}, nil)
	log.Infof("global[%d] committing is successfully done.", gt.XID)

	return true, err
}

//将bs序列化后，给个自增的ID，塞到tc.callBackMessages里的chan里。再将自增ID存到tc.futures里，等待30s等其他gorouting处理
func (tc *TransactionCoordinator) branchCommit(bs *apis.BranchSession) (apis.BranchSession_BranchStatus, error) {
	request := &apis.BranchCommitRequest{
		XID:             bs.XID,
		BranchID:        bs.BranchID,
		ResourceID:      bs.ResourceID,
		LockKey:         bs.LockKey,
		BranchType:      bs.Type,
		ApplicationData: bs.ApplicationData,
	}

	content, err := types.MarshalAny(request)
	if err != nil {
		return bs.Status, err
	}

	message := &apis.BranchMessage{
		ID:                int64(tc.idGenerator.Inc()),
		BranchMessageType: apis.TypeBranchCommit,
		Message:           content,
	}

	//在tc.callBackMessages这个map里读取branch_table的addressing，如果读取到则返回，如果没读取到，则新建make(chan *apis.BranchMessage)并返回
	//应该是通过tc.callBackMessages这个sync.map与另一个gorouting通信，这边负责塞数据
	queue, _ := tc.callBackMessages.LoadOrStore(bs.Addressing, make(chan *apis.BranchMessage))
	q := queue.(chan *apis.BranchMessage)
	//往q这个chan里放入message，如果塞不进去，就返回
	//如果是新创建的chan，应该是塞不进去的，直接就default了，只有之前有的，有其他gorouting监听了，才能塞进去
	//这个应该在RM连接的时候就调用了 BranchCommunicate ，所以那边的gorouting已经启动了
	select {
	case q <- message:
	default:
		return bs.Status, err
	}

	resp := common2.NewMessageFuture(message)
	//存储到tc.futures这个sync.map里，应该是 BranchCommunicate 的gorouting收到来自RM的响应后，去查找对应message.ID的内容，然后把 resp.Done 发送一个true
	//这里有个时间差，如果RM那边响应很快，而这边卡了一下，就会导致RM那边响应已经回来了，这个futures.Store还没写进去，就会导致 BranchCommunicate 那边收到 RM响应后，
	//在futures里找不到对应的key，然后跳过，这边又再写进去，不就会导致后面的30s超时吗？？？这个tc.futures.Store放到case q <- message:前面是不是更好？？？
	tc.futures.Store(message.ID, resp)

	//30s后从tc.futures中删除这个本地原子自增的ID，然后报超时错误
	timer := time.NewTimer(tc.streamMessageTimeout)
	select {
	case <-timer.C:
		tc.futures.Delete(resp.ID)
		return bs.Status, fmt.Errorf("wait branch commit response timeout")
	case <-resp.Done:
		timer.Stop()
	}

	//消耗tc.futures的gorouting应该会将里面的value的resp.Response塞入apis.BranchCommitResponse数据
	//也就是两个gorouting通过tc.futures这个sync.map传递消息
	response, ok := resp.Response.(*apis.BranchCommitResponse)
	if !ok {
		log.Infof("rollback response: %v", resp.Response)
		return bs.Status, fmt.Errorf("response type not right")
	}
	if response.ResultCode == apis.ResultCodeSuccess {
		//当RM响应成功后，会将状态返回
		return response.BranchStatus, nil
	}
	return bs.Status, fmt.Errorf(response.Message)
}

func (tc *TransactionCoordinator) Rollback(ctx context.Context, request *apis.GlobalRollbackRequest) (*apis.GlobalRollbackResponse, error) {
	gt := tc.holder.FindGlobalTransaction(request.XID)
	if gt == nil {
		return &apis.GlobalRollbackResponse{
			ResultCode:   apis.ResultCodeSuccess,
			GlobalStatus: apis.Finished,
		}, nil
	}
	shouldRollBack, err := func(gt *model.GlobalTransaction) (bool, error) {
		result, err := tc.locker.TryLock(gt.GlobalSession, time.Duration(gt.Timeout)*time.Millisecond)
		if err != nil {
			return false, err
		}
		if result {	//必定进这里
			defer tc.locker.Unlock(gt.GlobalSession)
			if gt.Active {
				// Active need persistence
				// Highlight: Firstly, close the session, then no more branch can be registered.
				err = tc.holder.InactiveGlobalSession(gt.GlobalSession)
				if err != nil {
					return false, err
				}
			}
			if gt.Status == apis.Begin {
				err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.RollingBack)
				if err != nil {
					return false, err
				}
				return true, nil
			}
			return false, nil
		}
		return false, fmt.Errorf("failed to lock global transaction xid = %s", request.XID)
	}(gt)

	if err != nil {
		return &apis.GlobalRollbackResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.FailedLockGlobalTransaction,
			Message:       err.Error(),
			GlobalStatus:  gt.Status,
		}, nil
	}

	if !shouldRollBack {	//gt.Status 进来的时候不是Begin
		return &apis.GlobalRollbackResponse{
			ResultCode:   apis.ResultCodeSuccess,
			GlobalStatus: gt.Status,
		}, nil
	}

	//回滚的时候就没有判断 gt.CanBeCommittedAsync() 了

	_, err = tc.doGlobalRollback(gt, false)
	if err != nil {
		return nil, err
	}
	return &apis.GlobalRollbackResponse{
		ResultCode:   apis.ResultCodeSuccess,
		GlobalStatus: gt.Status,
	}, nil
}

//跟 doGlobalCommit 有点像
func (tc *TransactionCoordinator) doGlobalRollback(gt *model.GlobalTransaction, retrying bool) (bool, error) {
	var err error

	runtime.GoWithRecover(func() {
		evt := event.NewGlobalTransactionEvent(gt.TransactionID, event.RoleTC, gt.TransactionName, gt.BeginTime, 0, gt.Status)
		event.EventBus.GlobalTransactionEventChannel <- evt
	}, nil)

	if gt.IsSaga() {
		return false, status.Errorf(codes.Unimplemented, "method Commit not supported saga mode")
	}

	for bs := range gt.BranchSessions {
		if bs.Status == apis.PhaseOneFailed {
			tc.resourceDataLocker.ReleaseLock(bs)
			delete(gt.BranchSessions, bs)
			err = tc.holder.RemoveBranchSession(gt.GlobalSession, bs)
			if err != nil {
				return false, err
			}
			continue
		}
		branchStatus, err1 := tc.branchRollback(bs)
		if err1 != nil {
			log.Errorf("exception rolling back branch xid=%d branchID=%d, err: %v", gt.XID, bs.BranchID, err1)
			if !retrying {
				if gt.IsTimeoutGlobalStatus() {
					err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.TimeoutRollbackRetrying)
					if err != nil {
						return false, err
					}
				} else {
					err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.RollbackRetrying)
					if err != nil {
						return false, err
					}
				}
			}
			return false, err1
		}
		switch branchStatus {
		case apis.PhaseTwoRolledBack:
			tc.resourceDataLocker.ReleaseLock(bs)
			delete(gt.BranchSessions, bs)
			err = tc.holder.RemoveBranchSession(gt.GlobalSession, bs)
			if err != nil {
				return false, err
			}
			log.Infof("successfully rollback branch xid=%d branchID=%d", gt.XID, bs.BranchID)
			continue
		case apis.PhaseTwoRollbackFailedCanNotRetry:
			if gt.IsTimeoutGlobalStatus() {
				err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.TimeoutRollbackFailed)
				if err != nil {
					return false, err
				}
			} else {
				err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.RollbackFailed)
				if err != nil {
					return false, err
				}
			}
			tc.resourceDataLocker.ReleaseGlobalSessionLock(gt)
			err = tc.holder.RemoveGlobalTransaction(gt)
			if err != nil {
				return false, err
			}
			log.Infof("failed to rollback branch and stop retry xid=%d branchID=%d", gt.XID, bs.BranchID)
			return false, nil
		default:
			log.Infof("failed to rollback branch xid=%d branchID=%d", gt.XID, bs.BranchID)
			if !retrying {
				if gt.IsTimeoutGlobalStatus() {
					err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.TimeoutRollbackRetrying)
					if err != nil {
						return false, err
					}
				} else {
					err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.RollbackRetrying)
					if err != nil {
						return false, err
					}
				}
			}
			return false, nil
		}
	}

	// In db mode, there is a problem of inconsistent data in multiple copies, resulting in new branch
	// transaction registration when rolling back.
	// 1. New branch transaction and rollback branch transaction have no data association
	// 2. New branch transaction has data association with rollback branch transaction
	// The second query can solve the first problem, and if it is the second problem, it may cause a rollback
	// failure due to data changes.
	gs := tc.holder.FindGlobalTransaction(gt.XID)
	if gs != nil && gs.HasBranch() {
		log.Infof("Global[%d] rolling back is NOT done.", gt.XID)
		return false, nil
	}

	//回滚分为超时回滚，这个要在最终状态写清楚
	if gt.IsTimeoutGlobalStatus() {
		err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.TimeoutRolledBack)
		if err != nil {
			return false, err
		}
	} else {
		err = tc.holder.UpdateGlobalSessionStatus(gt.GlobalSession, apis.RolledBack)
		if err != nil {
			return false, err
		}
	}
	tc.resourceDataLocker.ReleaseGlobalSessionLock(gt)
	err = tc.holder.RemoveGlobalTransaction(gt)
	if err != nil {
		return false, err
	}
	runtime.GoWithRecover(func() {
		evt := event.NewGlobalTransactionEvent(gt.TransactionID, event.RoleTC, gt.TransactionName, gt.BeginTime,
			int64(time2.CurrentTimeMillis()), gt.Status)
		event.EventBus.GlobalTransactionEventChannel <- evt
	}, nil)
	log.Infof("successfully rollback global, xid = %d", gt.XID)

	return true, err
}

//跟 branchCommit 比较像
func (tc *TransactionCoordinator) branchRollback(bs *apis.BranchSession) (apis.BranchSession_BranchStatus, error) {
	request := &apis.BranchRollbackRequest{
		XID:             bs.XID,
		BranchID:        bs.BranchID,
		ResourceID:      bs.ResourceID,
		LockKey:         bs.LockKey,
		BranchType:      bs.Type,
		ApplicationData: bs.ApplicationData,
	}

	content, err := types.MarshalAny(request)
	if err != nil {
		return bs.Status, err
	}
	message := &apis.BranchMessage{
		ID:                int64(tc.idGenerator.Inc()),
		BranchMessageType: apis.TypeBranchRollback,	//跟 branchCommit 区别是这里类型不一样
		Message:           content,
	}

	//也是写到tc.callBackMessages sync.map里，等其他取，是BranchCommunicate函数，看到是个grpc与RM stream通信的
	queue, _ := tc.callBackMessages.LoadOrStore(bs.Addressing, make(chan *apis.BranchMessage))
	q := queue.(chan *apis.BranchMessage)
	select {
	case q <- message:
	default:
		return bs.Status, err
	}

	resp := common2.NewMessageFuture(message)
	//放到tc.futures里，也是给 BranchCommunicate 函数，跟RM通信的
	tc.futures.Store(message.ID, resp)

	timer := time.NewTimer(tc.streamMessageTimeout)
	select {
	case <-timer.C:
		tc.futures.Delete(resp.ID)
		timer.Stop()
		return bs.Status, fmt.Errorf("wait branch rollback response timeout")
	case <-resp.Done:
		timer.Stop()
	}

	//等BranchCommunicate传true到resp.Done里，这里以后可能还会用到，所有没有close，虽然close也可以触发
	response := resp.Response.(*apis.BranchRollbackResponse)
	if response.ResultCode == apis.ResultCodeSuccess {
		return response.BranchStatus, nil
	}
	return bs.Status, fmt.Errorf(response.Message)
}

func (tc *TransactionCoordinator) BranchCommunicate(stream apis.ResourceManagerService_BranchCommunicateServer) error {
	var addressing string
	done := make(chan bool)

	ctx := stream.Context()
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		addressing = md.Get("addressing")[0]
		//activeApplications 记录每个 addressing 连接上来的个数，这个addressing就是sample里配的orderSvc或productSvc
		c, ok := tc.activeApplications.Load(addressing)
		if ok {
			count := c.(int)
			tc.activeApplications.Store(addressing, count+1)
		} else {
			tc.activeApplications.Store(addressing, 1)
		}
		defer func() {
			c, _ := tc.activeApplications.Load(addressing)
			count := c.(int)
			tc.activeApplications.Store(addressing, count-1)
		}()
	}

	//这个chan是 branchCommit 或 branchRollback 时，往这里塞数据，让这边取出后通过 BranchCommunicate 的stream发给RM的
	queue, _ := tc.callBackMessages.LoadOrStore(addressing, make(chan *apis.BranchMessage))
	q := queue.(chan *apis.BranchMessage)

	//启动gorouting，接收来自chan callBackMessages[addressing] 的 BranchMessage 数据，然后发给调用了 BranchCommunicate 协议连接上来的rm
	runtime.GoWithRecover(func() {
		for {
			select {
			case _, ok := <-done:
				if !ok {
					return
				}
			case msg := <- q:
				err := stream.Send(msg)
				if err != nil {
					return
				}
			}
		}
	}, nil)

	for {
		select {
		case <-ctx.Done():
			close(done)
			return ctx.Err()
		default:
			branchMessage, err := stream.Recv()
			if err == io.EOF {
				close(done)
				return nil
			}
			if err != nil {
				close(done)
				return err
			}
			switch branchMessage.GetBranchMessageType() {
			//不管收到啥，都删 tc.futures 对应的ID，如果rm回滚失败，则不返回
			case apis.TypeBranchCommitResult:
				response := &apis.BranchCommitResponse{}
				data := branchMessage.GetMessage().GetValue()
				err := response.Unmarshal(data)
				if err != nil {
					log.Error(err)
					continue
				}
				resp, loaded := tc.futures.Load(branchMessage.ID)
				if loaded {
					future := resp.(*common2.MessageFuture)
					future.Response = response
					future.Done <- true
					tc.futures.Delete(branchMessage.ID)
				}
			case apis.TypeBranchRollBackResult:
				response := &apis.BranchRollbackResponse{}
				data := branchMessage.GetMessage().GetValue()
				err := response.Unmarshal(data)
				if err != nil {
					log.Error(err)
					continue
				}
				resp, loaded := tc.futures.Load(branchMessage.ID)
				if loaded {
					future := resp.(*common2.MessageFuture)
					future.Response = response
					future.Done <- true
					tc.futures.Delete(branchMessage.ID)
				}
			}
		}
	}
}

func (tc *TransactionCoordinator) BranchRegister(ctx context.Context, request *apis.BranchRegisterRequest) (*apis.BranchRegisterResponse, error) {
	gt := tc.holder.FindGlobalTransaction(request.XID)
	if gt == nil {
		log.Errorf("could not found global transaction xid = %s", request.XID)
		return &apis.BranchRegisterResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.GlobalTransactionNotExist,
			Message:       fmt.Sprintf("could not found global transaction xid = %s", request.XID),
		}, nil
	}

	result, err := tc.locker.TryLock(gt.GlobalSession, time.Duration(gt.Timeout)*time.Millisecond)
	if err != nil {	//不会进这里
		return &apis.BranchRegisterResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.FailedLockGlobalTransaction,
			Message:       fmt.Sprintf("could not found global transaction xid = %s", request.XID),
		}, nil
	}
	if result {	//必定进这里
		defer tc.locker.Unlock(gt.GlobalSession)
		if !gt.Active {
			return &apis.BranchRegisterResponse{
				ResultCode:    apis.ResultCodeFailed,
				ExceptionCode: apis.GlobalTransactionNotActive,
				Message:       fmt.Sprintf("could not register branch into global session xid = %s status = %d", gt.XID, gt.Status),
			}, nil
		}
		if gt.Status != apis.Begin {
			return &apis.BranchRegisterResponse{
				ResultCode:    apis.ResultCodeFailed,
				ExceptionCode: apis.GlobalTransactionStatusInvalid,
				Message: fmt.Sprintf("could not register branch into global session xid = %s status = %d while expecting %d",
					gt.XID, gt.Status, apis.Begin),
			}, nil
		}

		bs := &apis.BranchSession{
			Addressing:      request.Addressing,
			XID:             request.XID,
			BranchID:        uuid.NextID(),
			TransactionID:   gt.TransactionID,
			ResourceID:      request.ResourceID,
			LockKey:         request.LockKey,
			Type:            request.BranchType,
			Status:          apis.Registered,
			ApplicationData: request.ApplicationData,
		}

		if bs.Type == apis.AT {
			//将请求的 LockKey 拆分为 RowLock 后，从数据库里判断是否被其他XID占用，如果有被其他XID占用的，就不能锁成功，否则将自己的XID和这些锁写入数据库中，之前已经有的当前XID的锁保持不变
			result := tc.resourceDataLocker.AcquireLock(bs)
			if !result {
				return &apis.BranchRegisterResponse{
					ResultCode:    apis.ResultCodeFailed,
					ExceptionCode: apis.LockKeyConflict,
					Message: fmt.Sprintf("branch lock acquire failed xid = %s resourceId = %s, lockKey = %s",
						request.XID, request.ResourceID, request.LockKey),
				}, nil
			}
		}

		//插入 branch_table 表，在mysql中 gt.GlobalSession 参数无用
		err := tc.holder.AddBranchSession(gt.GlobalSession, bs)
		if err != nil {
			log.Error(err)
			return &apis.BranchRegisterResponse{
				ResultCode:    apis.ResultCodeFailed,
				ExceptionCode: apis.BranchRegisterFailed,
				Message:       fmt.Sprintf("branch register failed, xid = %s, branchID = %d, err: %s", gt.XID, bs.BranchID, err.Error()),
			}, nil
		}

		return &apis.BranchRegisterResponse{
			ResultCode: apis.ResultCodeSuccess,
			BranchID:   bs.BranchID,
		}, nil
	}

	return &apis.BranchRegisterResponse{
		ResultCode:    apis.ResultCodeFailed,
		ExceptionCode: apis.FailedLockGlobalTransaction,
		Message:       fmt.Sprintf("failed to lock global transaction xid = %s", request.XID),
	}, nil
}

func (tc *TransactionCoordinator) BranchReport(ctx context.Context, request *apis.BranchReportRequest) (*apis.BranchReportResponse, error) {
	gt := tc.holder.FindGlobalTransaction(request.XID)
	if gt == nil {
		log.Errorf("could not found global transaction xid = %s", request.XID)
		return &apis.BranchReportResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.GlobalTransactionNotExist,
			Message:       fmt.Sprintf("could not found global transaction xid = %s", request.XID),
		}, nil
	}

	bs := gt.GetBranch(request.BranchID)
	if bs == nil {
		return &apis.BranchReportResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.BranchTransactionNotExist,
			Message:       fmt.Sprintf("could not found branch session xid = %s branchID = %d", gt.XID, request.BranchID),
		}, nil
	}

	//修改 branch_table 的 status 字段
	err := tc.holder.UpdateBranchSessionStatus(bs, request.BranchStatus)
	if err != nil {
		return &apis.BranchReportResponse{
			ResultCode:    apis.ResultCodeFailed,
			ExceptionCode: apis.BranchReportFailed,
			Message:       fmt.Sprintf("branch report failed, xid = %s, branchID = %d, err: %s", gt.XID, bs.BranchID, err.Error()),
		}, nil
	}

	return &apis.BranchReportResponse{
		ResultCode: apis.ResultCodeSuccess,
	}, nil
}

func (tc *TransactionCoordinator) LockQuery(ctx context.Context, request *apis.GlobalLockQueryRequest) (*apis.GlobalLockQueryResponse, error) {
	result := tc.resourceDataLocker.IsLockable(request.XID, request.ResourceID, request.LockKey)
	return &apis.GlobalLockQueryResponse{
		ResultCode: apis.ResultCodeSuccess,
		Lockable:   result,
	}, nil
}

func (tc *TransactionCoordinator) processTimeoutCheck() {
	for {
		timer := time.NewTimer(tc.timeoutRetryPeriod)

		<-timer.C
		tc.timeoutCheck()

		timer.Stop()
	}
}

func (tc *TransactionCoordinator) processRetryRollingBack() {
	for {
		timer := time.NewTimer(tc.rollingBackRetryPeriod)

		<-timer.C
		tc.handleRetryRollingBack()

		timer.Stop()
	}
}

func (tc *TransactionCoordinator) processRetryCommitting() {
	for {
		timer := time.NewTimer(tc.committingRetryPeriod)

		<-timer.C
		tc.handleRetryCommitting()

		timer.Stop()
	}
}

func (tc *TransactionCoordinator) processAsyncCommitting() {
	for {//默认配置1s
		timer := time.NewTimer(tc.asyncCommittingRetryPeriod)

		<-timer.C
		tc.handleAsyncCommitting()

		timer.Stop()
	}
}

//查表global_table，找出gmt_modified前100个，status为Begin的所有数据，这些数据中，如果已经超时了，就把active=0，status=TimeoutRollingBack，
//然后扔到 event.EventBus.GlobalTransactionEventChannel 管道中
func (tc *TransactionCoordinator) timeoutCheck() {
	//查表global_table，找出gmt_modified前100个，status为1的所有数据，放到GlobalSession结构里，GlobalSession与表global_table的结构一样，只是少了时间戳
	sessions := tc.holder.FindGlobalSessions([]apis.GlobalSession_GlobalStatus{apis.Begin})
	if len(sessions) == 0 {
		return
	}
	for _, globalSession := range sessions {
		if isGlobalSessionTimeout(globalSession) {	//如果当前时间-begin_time>timeout，就是已经超时了
			result, err := tc.locker.TryLock(globalSession, time.Duration(globalSession.Timeout)*time.Millisecond)	//空实现，必返回true
			if err == nil && result {//必定执行这里
				if globalSession.Active {
					// Active need persistence
					// Highlight: Firstly, close the session, then no more branch can be registered.
					err = tc.holder.InactiveGlobalSession(globalSession)	//将global_table当前xid对应表里的active=0，gmt_modified=now
					if err != nil {
						return
					}
				}
				//将表global_table里为xid的那行status=TimeoutRollingBack，gmt_modified=now
				//同时修改globalSession对象的状态
				err = tc.holder.UpdateGlobalSessionStatus(globalSession, apis.TimeoutRollingBack)
				if err != nil {
					return
				}

				tc.locker.Unlock(globalSession)	//空实现
				//返回一个GlobalTransactionEvent对象
				evt := event.NewGlobalTransactionEvent(globalSession.TransactionID, event.RoleTC, globalSession.TransactionName, globalSession.BeginTime, 0, globalSession.Status)
				event.EventBus.GlobalTransactionEventChannel <- evt
			}
		}
	}
}

func (tc *TransactionCoordinator) handleRetryRollingBack() {
	addressingIdentities := tc.getAddressingIdentities()
	if len(addressingIdentities) == 0 {
		return
	}
	//RollingBack, RollbackRetrying, TimeoutRollingBack, TimeoutRollbackRetrying，找到这些状态组成的GlobalTransaction切片
	rollbackTransactions := tc.holder.FindRetryRollbackGlobalTransactions(addressingIdentities)
	if len(rollbackTransactions) == 0 {
		return
	}
	now := time2.CurrentTimeMillis()
	for _, transaction := range rollbackTransactions {
		if transaction.Status == apis.RollingBack && !transaction.IsRollingBackDead() { //不大于12s？？？
			continue
		}
		//默认配置maxRollbackRetryTimeout为-1，不会进这里
		if isRetryTimeout(int64(now), tc.maxRollbackRetryTimeout, transaction.BeginTime) {
			if tc.rollbackRetryTimeoutUnlockEnable {
				tc.resourceDataLocker.ReleaseGlobalSessionLock(transaction)
			}
			err := tc.holder.RemoveGlobalTransaction(transaction)
			if err != nil {
				log.Error(err)
			}
			log.Errorf("GlobalSession rollback retry timeout and removed [%s]", transaction.XID)
			continue
		}
		_, err := tc.doGlobalRollback(transaction, true)
		if err != nil {
			log.Errorf("failed to retry rollback [%s]", transaction.XID)
		}
	}
}

func isRetryTimeout(now int64, timeout int64, beginTime int64) bool {
	if timeout >= AlwaysRetryBoundary && now-beginTime > timeout {
		return true
	}
	return false
}

func (tc *TransactionCoordinator) handleRetryCommitting() {
	addressingIdentities := tc.getAddressingIdentities()
	if len(addressingIdentities) == 0 {
		return
	}
	//找到status为CommitRetrying的global_table和branch_table数据，组成的GlobalTransaction切片
	committingTransactions := tc.holder.FindRetryCommittingGlobalTransactions(addressingIdentities)
	if len(committingTransactions) == 0 {
		return
	}
	now := time2.CurrentTimeMillis()
	for _, transaction := range committingTransactions {
		//默认配置设置成了-1，也就是这里是false
		if isRetryTimeout(int64(now), tc.maxCommitRetryTimeout, transaction.BeginTime) {
			err := tc.holder.RemoveGlobalTransaction(transaction)
			if err != nil {
				log.Error(err)
			}
			log.Errorf("GlobalSession commit retry timeout and removed [%s]", transaction.XID)
			continue
		}
		_, err := tc.doGlobalCommit(transaction, true)
		if err != nil {
			log.Errorf("failed to retry committing [%s]", transaction.XID)
		}
	}
}

func (tc *TransactionCoordinator) handleAsyncCommitting() {
	addressingIdentities := tc.getAddressingIdentities()	//将activeApplications map里 value>0的所有key组成一个切片返回
	if len(addressingIdentities) == 0 {
		return
	}
	//找到status是AsyncCommitting并且addressing为activeApplications里所有key的GlobalSession，
	//然后将包含这些GlobalSession对象的指针和与这个GlobalSession具有相同xid的BranchSession组成的指针的key的map，放到一个GlobalTransaction对象里
	//最后将这些对象合并成一个指针切片，返回。GlobalTransaction继承GlobalSession，多了相同xid的BranchSession切片
	asyncCommittingTransactions := tc.holder.FindAsyncCommittingGlobalTransactions(addressingIdentities)
	if len(asyncCommittingTransactions) == 0 {
		return
	}
	for _, transaction := range asyncCommittingTransactions {
		if transaction.Status != apis.AsyncCommitting {//再次筛选，防止出错
			continue
		}
		_, err := tc.doGlobalCommit(transaction, true)
		if err != nil {
			log.Errorf("failed to async committing [%s]", transaction.XID)
		}
	}
}

//获取tc.activeApplications这个map里的所有key
func (tc *TransactionCoordinator) getAddressingIdentities() []string {
	var addressIdentities []string
	tc.activeApplications.Range(func(key, value interface{}) bool {
		count := value.(int)
		if count > 0 {
			addressing := key.(string)
			addressIdentities = append(addressIdentities, addressing)
		}
		return true
	})
	return addressIdentities
}

func isGlobalSessionTimeout(gt *apis.GlobalSession) bool {
	//为啥不用 time.Now().UnixMilli() ？？？
	return (time2.CurrentTimeMillis() - uint64(gt.BeginTime)) > uint64(gt.Timeout)
}
