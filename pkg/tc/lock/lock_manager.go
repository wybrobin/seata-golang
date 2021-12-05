package lock

import (
	"github.com/opentrx/seata-golang/v2/pkg/apis"
	"github.com/opentrx/seata-golang/v2/pkg/tc/model"
	"github.com/opentrx/seata-golang/v2/pkg/tc/storage"
	"github.com/opentrx/seata-golang/v2/pkg/util/log"
)

type LockManager struct {
	manager storage.LockManager
}

func NewLockManager(manager storage.LockManager) *LockManager {
	return &LockManager{manager: manager}
}

func (locker *LockManager) AcquireLock(branchSession *apis.BranchSession) bool {
	if branchSession == nil {
		log.Debug("branchSession can't be null for memory/file locker.")
		return true
	}

	if branchSession.LockKey == "" {
		return true
	}

	locks := storage.CollectBranchSessionRowLocks(branchSession)
	if len(locks) == 0 {
		return true
	}

	//查询 locks 是否在数据库中被其他的XID占用，如果有一个被占用，就返回false，否则将这些 locks 标记为当前请求的XID占用
	return locker.manager.AcquireLock(locks)
}

//删除branch_table缓存在BranchSession对象里的一条记录里，通过resource_id和lock_key对应的所有lock_table里的记录
func (locker *LockManager) ReleaseLock(branchSession *apis.BranchSession) bool {
	if branchSession == nil {
		log.Debug("branchSession can't be null for memory/file locker.")
		return true
	}

	if branchSession.LockKey == "" {
		return true
	}

	locks := storage.CollectBranchSessionRowLocks(branchSession)
	if len(locks) == 0 {
		return true
	}

	return locker.manager.ReleaseLock(locks)
}

//删除globalTransaction对应的global_table记录下的所有branch_table的所有lock_table的记录
func (locker *LockManager) ReleaseGlobalSessionLock(globalTransaction *model.GlobalTransaction) bool {
	locks := make([]*apis.RowLock, 0)
	for branchSession := range globalTransaction.BranchSessions {
		rowLocks := storage.CollectBranchSessionRowLocks(branchSession)
		locks = append(locks, rowLocks...)
	}
	return locker.manager.ReleaseLock(locks)
}

//查表lock_table，看 lockKey ，这个以格式{tableName}:{pk1},{pk2},{pk3}...;{tableName}:{pk1},{pk2},{pk3}...;{tableName}:{pk1},{pk2},{pk3}...;...
//组成的字符串里的主键是否被其他XID锁住
func (locker *LockManager) IsLockable(xid string, resourceID string, lockKey string) bool {
	return locker.manager.IsLockable(xid, resourceID, lockKey)
}
