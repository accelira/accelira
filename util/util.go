package util

import (
	"encoding/base64"
	"fmt"
)

func Repeat(char rune, n int) string {
	result := make([]rune, n)
	for i := range result {
		result[i] = char
	}
	return string(result)
}

func Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func DisplayLogo() {
	logo := `
+===================================+
|    _                _ _           |
|   / \   ___ ___ ___| (_)_ __ __ _ |
|  / _ \ / __/ __/ _ \ | | '__/ _` + "`" + ` ||
| / ___ \ (_| (_|  __/ | | | | (_| ||
|/_/   \_\___\___\___|_|_|_|  \__,_||
+===================================+
`
	fmt.Print(logo)
}
