package domain

import (
	"path/filepath"
	"sort"
	"strings"
)

var supportedAudioExtensions = map[string]bool{
	".mp3": true, ".m4a": true, ".wav": true, ".flac": true, ".ogg": true, ".aac": true, ".mpga": true, ".mpeg": true, ".webm": true,
}

var providerCompatibleAudioExtensions = map[string]bool{
	".mp3": true, ".mp4": true, ".mpeg": true, ".mpga": true, ".m4a": true, ".wav": true, ".webm": true, ".flac": true, ".ogg": true,
}

var supportedVideoExtensions = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true,
}

func SupportedInputExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return supportedAudioExtensions[ext] || supportedVideoExtensions[ext]
}

func IsVideoInput(path string) bool {
	return supportedVideoExtensions[strings.ToLower(filepath.Ext(path))]
}

func ProviderCompatibleAudioExtension(path string) bool {
	return providerCompatibleAudioExtensions[strings.ToLower(filepath.Ext(path))]
}

func SupportedAudioExtensions() []string {
	return sortedExtensionKeys(supportedAudioExtensions)
}

func SupportedVideoExtensions() []string {
	return sortedExtensionKeys(supportedVideoExtensions)
}

func ProviderCompatibleAudioExtensions() []string {
	return sortedExtensionKeys(providerCompatibleAudioExtensions)
}

func sortedExtensionKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
