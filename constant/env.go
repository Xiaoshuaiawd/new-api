package constant

var StreamingTimeout int
var DifyDebug bool
var MaxFileDownloadMB int
var StreamScannerMaxBufferMB int
var ForceStreamOption bool
var CountToken bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var MaxRequestBodyMB int
var AzureDefaultAPIVersion string
var GeminiVisionMaxImageNum int
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
// PassThroughUpstreamError controls whether upstream error bodies/messages are returned to downstream clients.
// Default false for safety; enable via env `PASS_THROUGH_UPSTREAM_ERROR=true` when you need detailed upstream errors.
var PassThroughUpstreamError bool

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string
