package controlplane

import (
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func validateChannelCapacity(rec *storage.ChannelRecord) error {
	for _, protocol := range rec.Protocols {
		if model.IsRetiredProtocol(protocol) {
			return &ValidationError{Message: "旧版 Text Completions 已移除，请配置 Chat Completions 或 Responses"}
		}
	}
	if rec.MaxConcurrency < 0 || rec.MaxConcurrency > 10000 {
		return &ValidationError{Message: "每副本最大在途请求数须在 0–10000 之间，0 表示不限"}
	}
	if rec.QueueLimit < 0 || rec.QueueLimit > 10000 {
		return &ValidationError{Message: "上游等待队列长度须在 0–10000 之间，0 表示不排队"}
	}
	if rec.QueueWaitSeconds < 0 || rec.QueueWaitSeconds > 300 {
		return &ValidationError{Message: "上游最大排队时间须在 0–300 秒之间，0 使用默认 5 秒"}
	}
	return nil
}
