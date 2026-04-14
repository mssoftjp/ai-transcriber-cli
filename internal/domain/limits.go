package domain

const (
	ProviderUploadLimitBytes     int64   = 25 * 1024 * 1024
	IntermediateAudioBitrateBPS  float64 = 64_000
	IntermediateAudioBytesPerSec float64 = IntermediateAudioBitrateBPS / 8
)
