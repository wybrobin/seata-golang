package common

import (
	"fmt"
	"strconv"
	"strings"
)

func GenerateXID(addressing string, tranID int64) string {
	return fmt.Sprintf("%s:%d", addressing, tranID)
}

func GetTransactionID(xid string) int64 {
	if xid == "" {
		return -1
	}

	idx := strings.LastIndex(xid, ":")
	if len(xid) == idx+1 {//相当于最后一个是:
		return -1
	}
	//将xid中:后面的值变成64位十进制整型保存到tranID
	tranID, _ := strconv.ParseInt(xid[idx+1:], 10, 64)
	return tranID
}
