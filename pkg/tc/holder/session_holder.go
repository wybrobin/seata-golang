package holder

import (
	"github.com/opentrx/seata-golang/v2/pkg/apis"
	"github.com/opentrx/seata-golang/v2/pkg/tc/model"
	"github.com/opentrx/seata-golang/v2/pkg/tc/storage"
)

type SessionHolder struct {
	manager storage.SessionManager
}

func NewSessionHolder(manager storage.SessionManager) *SessionHolder {
	return &SessionHolder{manager: manager}
}

func (holder *SessionHolder) AddGlobalSession(session *apis.GlobalSession) error {
	return holder.manager.AddGlobalSession(session)
}

func (holder *SessionHolder) FindGlobalSession(xid string) *apis.GlobalSession {
	return holder.manager.FindGlobalSession(xid)
}

//将xid的global_table数据，与它下面具有相同xid的branch_table的数据组成的map，组合成GlobalTransaction对象
func (holder *SessionHolder) FindGlobalTransaction(xid string) *model.GlobalTransaction {
	globalSession := holder.manager.FindGlobalSession(xid)
	if globalSession != nil {
		gt := &model.GlobalTransaction{GlobalSession: globalSession}
		branchSessions := holder.manager.FindBranchSessions(xid)
		if len(branchSessions) != 0 {
			gt.BranchSessions = make(map[*apis.BranchSession]bool, len(branchSessions))
			for i := 0; i < len(branchSessions); i++ {
				gt.BranchSessions[branchSessions[i]] = true
			}
		}
		return gt
	}
	return nil
}

func (holder *SessionHolder) FindAsyncCommittingGlobalTransactions(addressingIdentities []string) []*model.GlobalTransaction {
	return holder.findGlobalTransactionsWithAddressingIdentities([]apis.GlobalSession_GlobalStatus{
		apis.AsyncCommitting,
	}, addressingIdentities)
}

func (holder *SessionHolder) FindRetryCommittingGlobalTransactions(addressingIdentities []string) []*model.GlobalTransaction {
	return holder.findGlobalTransactionsWithAddressingIdentities([]apis.GlobalSession_GlobalStatus{
		apis.CommitRetrying,
	}, addressingIdentities)
}

func (holder *SessionHolder) FindRetryRollbackGlobalTransactions(addressingIdentities []string) []*model.GlobalTransaction {
	return holder.findGlobalTransactionsWithAddressingIdentities([]apis.GlobalSession_GlobalStatus{
		apis.RollingBack, apis.RollbackRetrying, apis.TimeoutRollingBack, apis.TimeoutRollbackRetrying,
	}, addressingIdentities)
}

func (holder *SessionHolder) findGlobalTransactions(statuses []apis.GlobalSession_GlobalStatus) []*model.GlobalTransaction {
	gts := holder.manager.FindGlobalSessions(statuses)
	return holder.findGlobalTransactionsByGlobalSessions(gts)
}

//找到状态为statuses的global_table和branch_table组成的数据
func (holder *SessionHolder) findGlobalTransactionsWithAddressingIdentities(statuses []apis.GlobalSession_GlobalStatus,
	addressingIdentities []string) []*model.GlobalTransaction {
	//找到表global_table里，status in statuses and addressing in addressingIdentities 的 GlobalSession，也就是global_table表数据
	gts := holder.manager.FindGlobalSessionsWithAddressingIdentities(statuses, addressingIdentities)
	return holder.findGlobalTransactionsByGlobalSessions(gts)
}

//查表，将具有相同xid的branch放到一起，组成一个GlobalTransaction，然后把这些GlobalTransaction返回
func (holder *SessionHolder) findGlobalTransactionsByGlobalSessions(sessions []*apis.GlobalSession) []*model.GlobalTransaction {
	if len(sessions) == 0 {
		return nil
	}

	//所有XID放到xids里
	xids := make([]string, 0, len(sessions))
	for _, gt := range sessions {
		xids = append(xids, gt.XID)
	}
	//从表branch_table里找到所有xid in xids的数据，放到branchSessions里，BranchSession的结构对应branch_table，跟GlobalSession一样，只是去掉了创建修改时间
	branchSessions := holder.manager.FindBatchBranchSessions(xids)
	branchSessionMap := make(map[string][]*apis.BranchSession)
	//将branchSessions归类，相同的XID放到一起，组成一个XID映射branchSessions切片的map
	for i := 0; i < len(branchSessions); i++ {
		branchSessionSlice, ok := branchSessionMap[branchSessions[i].XID]
		if ok {
			branchSessionSlice = append(branchSessionSlice, branchSessions[i])
			branchSessionMap[branchSessions[i].XID] = branchSessionSlice
		} else {
			branchSessionSlice = make([]*apis.BranchSession, 0)
			branchSessionSlice = append(branchSessionSlice, branchSessions[i])
			branchSessionMap[branchSessions[i].XID] = branchSessionSlice
		}
	}

	//将包含GlobalSession对象的指针和与这个GlobalSession具有相同xid的BranchSession组成的指针的key的map，放到一个GlobalTransaction对象里
	//然后将这些对象合并成一个指针切片，返回
	globalTransactions := make([]*model.GlobalTransaction, 0, len(sessions))
	for j := 0; j < len(sessions); j++ {
		globalTransaction := &model.GlobalTransaction{
			GlobalSession:  sessions[j],
			BranchSessions: map[*apis.BranchSession]bool{},	//初始化了一个空map
		}

		branchSessionSlice := branchSessionMap[sessions[j].XID]
		if len(branchSessionSlice) > 0 {
			for x := 0; x < len(branchSessionSlice); x++ {
				globalTransaction.BranchSessions[branchSessionSlice[x]] = true
			}
		}
		globalTransactions = append(globalTransactions, globalTransaction)
	}

	return globalTransactions
}

func (holder *SessionHolder) FindGlobalSessions(statuses []apis.GlobalSession_GlobalStatus) []*apis.GlobalSession {
	return holder.manager.FindGlobalSessions(statuses)
}

func (holder *SessionHolder) AllSessions() []*apis.GlobalSession {
	return holder.manager.AllSessions()
}

//修改global_table状态
func (holder *SessionHolder) UpdateGlobalSessionStatus(session *apis.GlobalSession, status apis.GlobalSession_GlobalStatus) error {
	session.Status = status
	return holder.manager.UpdateGlobalSessionStatus(session, status)
}

func (holder *SessionHolder) InactiveGlobalSession(session *apis.GlobalSession) error {
	session.Active = false
	return holder.manager.InactiveGlobalSession(session)
}

func (holder *SessionHolder) RemoveGlobalSession(session *apis.GlobalSession) error {
	return holder.manager.RemoveGlobalSession(session)
}

//删除globalTransaction里对应的所有global_table和branch_table的记录
func (holder *SessionHolder) RemoveGlobalTransaction(globalTransaction *model.GlobalTransaction) error {
	err := holder.manager.RemoveGlobalSession(globalTransaction.GlobalSession)
	if err != nil {
		return err
	}
	for bs := range globalTransaction.BranchSessions {
		err = holder.manager.RemoveBranchSession(globalTransaction.GlobalSession, bs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (holder *SessionHolder) AddBranchSession(globalSession *apis.GlobalSession, session *apis.BranchSession) error {
	return holder.manager.AddBranchSession(globalSession, session)
}

func (holder *SessionHolder) FindBranchSession(xid string) []*apis.BranchSession {
	return holder.manager.FindBranchSessions(xid)
}

func (holder *SessionHolder) UpdateBranchSessionStatus(session *apis.BranchSession, status apis.BranchSession_BranchStatus) error {
	return holder.manager.UpdateBranchSessionStatus(session, status)
}

func (holder *SessionHolder) RemoveBranchSession(globalSession *apis.GlobalSession, session *apis.BranchSession) error {
	return holder.manager.RemoveBranchSession(globalSession, session)
}
