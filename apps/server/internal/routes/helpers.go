package routes

import "encoding/json"

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
