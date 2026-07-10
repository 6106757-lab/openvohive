package backend

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openvohive/openvohive/pkg/smscodec"
)

// ============================================================================
// MBIM 后端的短信收发 — 走 AT 命令（复用 modem.Manager）
// RM520N-GL 的 MBIM SMS_SEND 存在兼容性问题（始终返回 status=21），
// 但 AT 命令可以正常收发短信。这里直接委托给 modem.Manager 处理，
// 与 AT 模式保持一致。
// ============================================================================

func (b *MBIMBackend) SendSMS(ctx context.Context, to, body string) error {
	if b.modem == nil {
		return fmt.Errorf("AT 管理器未启动或不可用")
	}
	return b.modem.SendSMS(to, body)
}

func (b *MBIMBackend) SendSMSWithOptions(ctx context.Context, to, body string, opts smscodec.SubmitOptions) error {
	if b.modem == nil {
		return fmt.Errorf("AT 管理器未启动或不可用")
	}
	return b.modem.SendSMSWithOptions(to, body, opts)
}

func (b *MBIMBackend) ReadSMS(ctx context.Context, index int) (*SMS, error) {
	if b.modem == nil {
		return nil, fmt.Errorf("AT 管理器未启动或不可用")
	}
	// 委托给 modem 读取 PDU
	pdu, err := b.modem.SMSReadPDU(fmt.Sprintf("%d", index))
	if err != nil {
		return nil, err
	}
	if pdu == "" {
		return nil, fmt.Errorf("短信 %d 不存在或为空", index)
	}
	return &SMS{Index: index, Content: pdu}, nil
}

func (b *MBIMBackend) DeleteSMS(ctx context.Context, index int) error {
	if b.modem == nil {
		return fmt.Errorf("AT 管理器未启动或不可用")
	}
	_, err := b.modem.ExecuteAT(fmt.Sprintf("AT+CMGD=%d", index), 10*time.Second)
	return err
}

func (b *MBIMBackend) ListSMS(ctx context.Context) ([]SMSSummary, error) {
	if b.modem == nil {
		return nil, fmt.Errorf("AT 管理器未启动或不可用")
	}
	// 用 AT+CMGL=4 列出所有 PDU
	resp, err := b.modem.ExecuteAT("AT+CMGL=4", 15*time.Second)
	if err != nil {
		return nil, err
	}

	var out []SMSSummary
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CMGL:") {
			continue
		}
		// 格式: +CMGL: <index>,<stat>,...
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(parts[0][6:])) // strip "+CMGL: "
		if err != nil {
			continue
		}
		stat, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			stat = 0
		}
		out = append(out, SMSSummary{Index: idx, Tag: stat})
	}
	return out, nil
}

func (b *MBIMBackend) DeleteAllSMS(ctx context.Context) error {
	if b.modem == nil {
		return fmt.Errorf("AT 管理器未启动或不可用")
	}
	return b.modem.SMSDeleteAll()
}

func (b *MBIMBackend) GetSMSC(ctx context.Context) (string, error) {
	if b.modem == nil {
		return "", fmt.Errorf("AT 管理器未启动或不可用")
	}
	resp, err := b.modem.ExecuteAT("AT+CSCA?", 5*time.Second)
	if err != nil {
		return "", err
	}
	// 解析 +CSCA: "+8613800839500",145
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CSCA:") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start >= 0 && end > start {
				return line[start+1 : end], nil
			}
		}
	}
	return "", fmt.Errorf("无法解析 SMSC")
}

func (b *MBIMBackend) SetSMSC(ctx context.Context, smsc string) error {
	if b.modem == nil {
		return fmt.Errorf("AT 管理器未启动或不可用")
	}
	_, err := b.modem.ExecuteAT(fmt.Sprintf("AT+CSCA=\"%s\"", smsc), 5*time.Second)
	return err
}
