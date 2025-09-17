package filter

import (
	"slices"
	"strings"

	"github.com/BooleanCat/go-functional/v2/it"
)

func RoutesByAppGUIDsPredicate(appGuids []string) func(value any) bool {
	return func(value any) bool {
		destinationAppGUIDsStr := value.(string)
		destinationAppGUIDs := strings.Split(destinationAppGUIDsStr, "\n")
		return haveCommonElements(appGuids, destinationAppGUIDs)
	}
}

func haveCommonElements(appGuids, destinationAppGUIDs []string) bool {
	return it.Any(it.Map(slices.Values(appGuids), func(appGUID string) bool {
		return slices.Contains(destinationAppGUIDs, appGUID)
	}))
}
