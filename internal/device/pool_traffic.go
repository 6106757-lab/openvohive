package device

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/openvohive/openvohive/internal/db"
	"github.com/openvohive/openvohive/pkg/logger"
	"github.com/openvohive/openvohive/pkg/quectel-qmi/qmi"
)

// trafficSample 记录上一采样周期累计收/发字节数, 用于计算分钟增量。
type trafficSample struct {
	rx uint64
	tx uint64
}

// runTrafficAggregation 是流量采集与聚合后台循环。
//
// 根因：此前流量采集器从未调用 db.UpsertTrafficMinute，traffic_minute 表恒为空，
// 导致 Web 流量分析页面全 0。这里每分钟采样一次 QMI 包统计（最准的蜂窝流量源，
// 绕开逻辑网口无计数器的问题），按增量写入 traffic_minute，并调度 hour/day 聚合，
// 使 Web 流量分析（day/week/month）与实时概览都有数据。
func (p *Pool) runTrafficAggregation() {
	logger.Info("流量采集器已启动")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// 对齐到整分钟边界
	time.Sleep(time.Until(time.Now().Add(time.Minute).Truncate(time.Minute)))

	for {
		select {
		case <-p.ctx.Done():
			logger.Info("流量采集器已停止")
			return
		case <-ticker.C:
			p.sampleTraffic()
			p.runRollups()
		}
	}
}

// sampleTraffic 对 Pool 中每个已连接 QMI worker 采样流量并写入 traffic_minute。
func (p *Pool) sampleTraffic() {
	p.mu.RLock()
	workers := make([]*Worker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.mu.RUnlock()

	now := time.Now()
	periodStart := now.Truncate(time.Minute)

	logger.Debug(fmt.Sprintf("[traffic] 采样开始 workers=%d time=%s", len(workers), now.Format("15:04:05")))

	for _, w := range workers {
		if w == nil {
			continue
		}
		// 优先从 /proc/net/dev 读取（最可靠，直接读网卡计数器）
		// QMI WDS 统计在某些场景下计数器不更新（始终为 0），不可靠
		rx, tx, ok := readWorkerTrafficFromProc(w)
		if !ok {
			logger.Debug(fmt.Sprintf("[traffic] %s proc fallback 失败", w.ID))
			continue
		}
		p.recordTrafficDelta(w, periodStart, rx, tx)
	}
}

func (p *Pool) recordTrafficDelta(w *Worker, periodStart time.Time, rx, tx uint64) {
	p.trafficPrevMu.Lock()
	prev, had := p.trafficPrev[w.ID]
	p.trafficPrev[w.ID] = trafficSample{rx: rx, tx: tx}
	p.trafficPrevMu.Unlock()

	if !had {
		// 首个采样点只记录基线，不下发（无增量可比）。
		logger.Debug(fmt.Sprintf("[traffic] %s 基线已建立 rx=%d tx=%d", w.ID, rx, tx))
		return
	}

	dRx := int64(rx - prev.rx)
	dTx := int64(tx - prev.tx)
	if dRx < 0 {
		dRx = 0
	}
	if dTx < 0 {
		dTx = 0
	}
	if dRx == 0 && dTx == 0 {
		return
	}

	iface := w.Config.Interface
	if iface == "" {
		iface = "wwan0"
	}
	tag := w.ID + "@" + iface

	if err := db.UpsertTrafficMinute([]db.TrafficPoint{
		{PeriodStart: periodStart, Resource: "iface", Tag: tag, Direction: false, TrafficBytes: dRx},
		{PeriodStart: periodStart, Resource: "iface", Tag: tag, Direction: true, TrafficBytes: dTx},
	}); err != nil {
		log.Printf("[traffic] upsert 失败 device=%s: %v", w.ID, err)
	} else {
		logger.Debug(fmt.Sprintf("[traffic] %s 写入 rx=%d tx=%d", w.ID, dRx, dTx))
	}
}

// readWorkerTraffic 通过 QMI 包统计读取累计收发字节数。
// 优先使用 wwan 数据呼叫的 Tx/Rx 字节数（最准确，且不受逻辑网口无计数器影响）。
func readWorkerTraffic(w *Worker) (rx, tx uint64, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const mask = qmi.WDSPacketStatsTxBytesOK | qmi.WDSPacketStatsRxBytesOK
	stats, err := w.QMICore.WDSGetPacketStatistics(ctx, mask)
	if err != nil {
		logger.Debug(fmt.Sprintf("[traffic] %s QMI WDSGetPacketStatistics 错误: %v", w.ID, err))
		return 0, 0, false
	}
	if stats == nil {
		logger.Debug(fmt.Sprintf("[traffic] %s QMI WDSGetPacketStatistics 返回 nil", w.ID))
		return 0, 0, false
	}
	logger.Debug(fmt.Sprintf("[traffic] %s QMI统计 rx=%d tx=%d", w.ID, stats.RxBytesOK, stats.TxBytesOK))
	return stats.RxBytesOK, stats.TxBytesOK, true
}

// runRollups 把 traffic_minute 聚合进 traffic_hour / traffic_day。
//
// Web 流量分析：day 读 traffic_hour(历史)+traffic_minute(当前)，
// week/month 读 traffic_day(历史)+traffic_hour(当前)，故只需 hour/day 两级聚合。
// 每次滚动当前与上一周期（幂等），保证整分钟/整点边界与进程重启后都能补齐。
func (p *Pool) runRollups() {
	now := time.Now()

	// hour 聚合：当前小时 + 上一小时
	if err := db.RollupToHour(now.Truncate(time.Hour)); err != nil {
		log.Printf("[traffic] rollup hour 失败: %v", err)
	}
	if err := db.RollupToHour(now.Add(-time.Hour).Truncate(time.Hour)); err != nil {
		log.Printf("[traffic] rollup prev hour 失败: %v", err)
	}

	// day 聚合：当天 + 前一天
	dayStart := startOfTrafficDay(now)
	if err := db.RollupToDay(dayStart); err != nil {
		log.Printf("[traffic] rollup day 失败: %v", err)
	}
	if err := db.RollupToDay(dayStart.AddDate(0, 0, -1)); err != nil {
		log.Printf("[traffic] rollup prev day 失败: %v", err)
	}
}

// startOfTrafficDay 取当日起点（本地 00:00）。
func startOfTrafficDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// readWorkerTrafficFromProc 从 /proc/net/dev 读取网卡流量计数器作为 fallback。
func readWorkerTrafficFromProc(w *Worker) (rx, tx uint64, ok bool) {
	iface := w.Config.Interface
	if iface == "" {
		iface = "wwan0"
	}
	data, err := readProcNetDev()
	if err != nil {
		return 0, 0, false
	}
	rx, tx = parseProcNetDev(data, iface)
	return rx, tx, true
}
