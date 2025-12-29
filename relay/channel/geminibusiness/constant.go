package geminibusiness

var ModelList = []string{
	"gemini-auto",
	"gemini-2.5-flash",
	"gemini-2.5-pro",
	"gemini-3-flash-preview",
	"gemini-3-pro-preview",
}

var modelMapping = map[string]string{
	"gemini-auto":            "",
	"gemini-2.5-flash":       "gemini-2.5-flash",
	"gemini-2.5-pro":         "gemini-2.5-pro",
	"gemini-3-flash-preview": "gemini-3-flash-preview",
	"gemini-3-pro-preview":   "gemini-3-pro-preview",
}

const ChannelName = "gemini-business"

const (
	businessBaseURL   = "https://business.gemini.google"
	discoveryBaseURL  = "https://biz-discoveryengine.googleapis.com"
	createSessionPath = "/v1alpha/locations/global/widgetCreateSession"
	addFilePath       = "/v1alpha/locations/global/widgetAddContextFile"
	streamAssistPath  = "/v1alpha/locations/global/widgetStreamAssist"
	getXsrfPath       = "/auth/getoxsrf"
)
