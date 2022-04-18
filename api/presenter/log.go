package presenter

import (
	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type LogCacheReadResponse struct {
	Envelopes LogCacheReadResponseEnvelopes `json:"envelopes"`
}

type LogCacheReadResponseEnvelopes struct {
	Batch []LogCacheReadResponseBatch `json:"batch"`
}

type LogCacheReadResponseBatch struct {
	Timestamp int64                   `json:"timestamp"`
	Log       LogCacheReadResponseLog `json:"log"`
}

type LogCacheReadResponseLog struct {
	Payload []byte                  `json:"payload"`
	Type    loggregator_v2.Log_Type `json:"type"`
}

func ForLogs(logRecords []repositories.LogRecord) LogCacheReadResponse {
	envelopes := make([]LogCacheReadResponseBatch, 0, len(logRecords))
	for _, logRecord := range logRecords {
		batch := LogCacheReadResponseBatch{
			Timestamp: logRecord.Timestamp,
			Log: LogCacheReadResponseLog{
				Payload: []byte(logRecord.Message),
				Type:    loggregator_v2.Log_OUT,
			},
		}

		envelopes = append(envelopes, batch)
	}

	return LogCacheReadResponse{
		Envelopes: LogCacheReadResponseEnvelopes{
			Batch: envelopes,
		},
	}
}
