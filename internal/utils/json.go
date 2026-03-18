package utils

import "encoding/json"

func EncodeJSON(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}
