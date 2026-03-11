package operation_setting

import "strings"

var DemoSiteEnabled = false
var SelfUseModeEnabled = false

// SubscriptionOnlyModeEnabled toggles wallet-based balance features off and enforces subscription-first UX.
var SubscriptionOnlyModeEnabled = false

// SubscriptionOnlyModeEnabled toggles wallet/balance features off and forces subscription-only billing UX
var SubscriptionOnlyModeEnabled = false

var AutomaticDisableKeywords = []string{
	"Your credit balance is too low",
	"This organization has been disabled.",
	"You exceeded your current quota",
	"Permission denied",
	"The security token included in the request is invalid",
	"Operation not allowed",
	"Your account is not authorized",
}

func AutomaticDisableKeywordsToString() string {
	return strings.Join(AutomaticDisableKeywords, "\n")
}

func AutomaticDisableKeywordsFromString(s string) {
	AutomaticDisableKeywords = []string{}
	ak := strings.Split(s, "\n")
	for _, k := range ak {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			AutomaticDisableKeywords = append(AutomaticDisableKeywords, k)
		}
	}
}
