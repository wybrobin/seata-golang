package storage

import (
	"fmt"
	"strings"

	"github.com/opentrx/seata-golang/v2/pkg/apis"
	"github.com/opentrx/seata-golang/v2/pkg/util/common"
)

const LockSplit = "^^^"

func CollectBranchSessionRowLocks(branchSession *apis.BranchSession) []*apis.RowLock {
	if branchSession == nil || branchSession.LockKey == "" {
		return nil
	}
	return collectRowLocks(branchSession.LockKey, branchSession.ResourceID, branchSession.XID, branchSession.TransactionID, branchSession.BranchID)
}

func CollectRowLocks(lockKey string, resourceID string, xid string) []*apis.RowLock {
	return collectRowLocks(lockKey, resourceID, xid, common.GetTransactionID(xid), 0)
}

//将branch_table表的lock_key字段通过;:,拆开，存到RowLock（对应表lock_table），组成切片返回
//为什么不用xid关联，要用这么麻烦的组合呢？？？为了少查一个lock_table表？就是只记录在branch_table就可以了，也不用了，直接对lock_table用xid筛选就完了啊？？？
//这个函数的lockKey参数不一定是branch_table表传过来的lock_key字段，这个函数只是将参数lockKey拆成TableName和pk
//从这个函数来看，lockKey的格式是：{tableName}:{pk1},{pk2},{pk3}...;{tableName}:{pk1},{pk2},{pk3}...;{tableName}:{pk1},{pk2},{pk3}...;...
//一个pk就是一个RowLock
func collectRowLocks(lockKey string,
	resourceID string,
	xid string,
	transactionID int64,
	branchID int64) []*apis.RowLock {
	var locks = make([]*apis.RowLock, 0)
	tableGroupedLockKeys := strings.Split(lockKey, ";")
	for _, tableGroupedLockKey := range tableGroupedLockKeys {
		if tableGroupedLockKey != "" {
			idx := strings.Index(tableGroupedLockKey, ":")
			if idx < 0 {
				return nil
			}

			tableName := tableGroupedLockKey[0:idx]
			mergedPKs := tableGroupedLockKey[idx+1:]

			if mergedPKs == "" {
				return nil
			}

			pks := strings.Split(mergedPKs, ",")
			if len(pks) == 0 {
				return nil
			}

			for _, pk := range pks {
				if pk != "" {
					//RowLock对应表lock_table
					rowLock := &apis.RowLock{
						XID:           xid,
						TransactionID: transactionID,
						BranchID:      branchID,
						ResourceID:    resourceID,
						TableName:     tableName,
						PK:            pk,
						RowKey:        getRowKey(resourceID, tableName, pk),	//用resourceID、tableName、pk组成一个主键，为什么不把这3个字段作为联合主键？
					}
					locks = append(locks, rowLock)
				}
			}
		}
	}
	return locks
}

func getRowKey(resourceID string, tableName string, pk string) string {
	return fmt.Sprintf("%s^^^%s^^^%s", resourceID, tableName, pk)
}
