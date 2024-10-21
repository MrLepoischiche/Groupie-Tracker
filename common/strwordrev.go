package common

func StrWordRev(s string) string {
	if len(s) <= 1 {
		return s
	}

	res := ""

	endWord := len(s) - 1

	for i := len(s) - 1; i >= 0; i-- {
		switch (s[i] < 'A' || s[i] > 'Z') && (s[i] < 'a' || s[i] > 'z') && (s[i] < '0' || s[i] > '9') {
		case true:
			if i >= endWord {
				endWord = endWord - 1
				continue
			}
			res += s[i+1:endWord+1] + string(s[i])
			endWord = i - 1
		default:
			if i == 0 {
				res += s[:endWord+1]
			}
		}
	}

	return res
}
