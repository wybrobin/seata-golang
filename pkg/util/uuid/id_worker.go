package uuid

import (
	"fmt"
	"math/rand"
	"net"
	"sync/atomic"

	time2 "github.com/opentrx/seata-golang/v2/pkg/util/time"
)

const (
	// Start time cut (2020-05-03)
	epoch uint64 = 1588435200000

	// The number of bits occupied by workerID
	workerIDBits = 10

	// The number of bits occupied by timestamp
	timestampBits = 41

	// The number of bits occupied by sequence
	sequenceBits = 12

	// Maximum supported machine id, the result is 1023
	maxWorkerID = -1 ^ (-1 << workerIDBits)

	// mask that help to extract timestamp and sequence from a long
	//0x1FFFFFFFFFFFFF
	timestampAndSequenceMask uint64 = -1 ^ (-1 << (timestampBits + sequenceBits))
)

// timestamp and sequence mix in one Long
// highest 11 bit: not used
// middle  41 bit: timestamp
// lowest  12 bit: sequence
var timestampAndSequence uint64

// business meaning: machine ID (0 ~ 1023)
// actual layout in memory:
// highest 1 bit: 0
// middle 10 bit: workerID
// lowest 53 bit: all 0
var workerID = generateWorkerID() << (timestampBits + sequenceBits)

func init() {
	timestamp := getNewestTimestamp()
	timestampWithSequence := timestamp << sequenceBits
	atomic.StoreUint64(&timestampAndSequence, timestampWithSequence)
}

//通过命令行-n参数指定的serverNode，1-1023，用来设置 workerID ，所以全局的 workerID tc没有用
func Init(serverNode int64) error {
	if serverNode > maxWorkerID || serverNode < 0 {
		return fmt.Errorf("worker id can't be greater than %d or less than 0", maxWorkerID)
	}
	workerID = serverNode << (timestampBits + sequenceBits)
	return nil
}

//有点类似雪花算法，低12位保存自增值，然后中间41位保存加工过的毫秒时间戳，剩下的高位保存workerID，由serverNode生成。
//也就是如果每毫秒生成数量超过4096，那么就会导致id重复
func NextID() int64 {
	next := atomic.AddUint64(&timestampAndSequence, 1)
	timestampWithSequence := next & timestampAndSequenceMask
	return int64(uint64(workerID) | timestampWithSequence)
}

// get newest timestamp relative to twepoch
func getNewestTimestamp() uint64 {
	return time2.CurrentTimeMillis() - epoch
}

// auto generate workerID, try using mac first, if failed, then randomly generate one
func generateWorkerID() int64 {
	id, err := generateWorkerIDBaseOnMac()
	if err != nil {
		id = generateRandomWorkerID()
	}
	return id
}

// use lowest 10 bit of available MAC as workerID
func generateWorkerIDBaseOnMac() (int64, error) {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		mac := iface.HardwareAddr

		return int64(int(rune(mac[4]&0b11)<<8) | int(mac[5]&0xFF)), nil
	}
	return 0, fmt.Errorf("no available mac found")
}

// randomly generate one as workerID
func generateRandomWorkerID() int64 {
	return rand.Int63n(maxWorkerID + 1)
}
