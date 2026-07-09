package device

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// procNetDevCache 缓存 /proc/net/dev 内容以避免频繁读取
var procNetDevCache = "/proc/net/dev"

func readProcNetDev() (string, error) {
	data, err := os.ReadFile(procNetDevCache)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseProcNetDev 解析 /proc/net/dev 中指定网卡的 rx/tx 字节数
func parseProcNetDev(content string, iface string) (rx, tx uint64) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		// 格式:  iface: rx_bytes rx_packets ... tx_bytes tx_packets ...
		if !strings.Contains(line, iface+":") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// fields[0] = "iface:" (带冒号)
		// fields[1] = rx_bytes
		// fields[9] = tx_bytes
		rxVal, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		txVal, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		return rxVal, txVal
	}
	return 0, 0
}

// procNetDevCache 缓存 /proc/net/dev 路径
