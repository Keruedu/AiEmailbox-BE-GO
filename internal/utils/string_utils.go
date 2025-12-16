package utils

import "strings"

// Levenshtein calculates the Levenshtein distance between two strings
func Levenshtein(s1, s2 string) int {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)
	r1, r2 := []rune(s1), []rune(s2)
	n, m := len(r1), len(r2)

	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	row := make([]int, m+1)
	for i := 0; i <= m; i++ {
		row[i] = i
	}

	for i := 1; i <= n; i++ {
		prev := i
		var val int
		for j := 1; j <= m; j++ {
			if r1[i-1] == r2[j-1] {
				val = row[j-1]
			} else {
				val = min(min(row[j-1]+1, prev+1), row[j]+1)
			}
			row[j-1] = prev
			prev = val
		}
		row[m] = prev
	}
	return row[m]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RemoveAccents removes Vietnamese accents from a string
func RemoveAccents(s string) string {
	// A basic mapping or using normalization libraries is needed.
	// Since we can't easily import external text/* libs if not already in go.mod and verified,
	// we'll implement a simple map-based replacer for common Vietnamese chars.
	// Or relies on runic decomposition if standard lib supports it well enough.
	// Let's use a simple mapping for reliability without adding heavy dependencies.

	// Map of accented chars to unaccented
	accents := map[rune]rune{
		'á': 'a', 'à': 'a', 'ả': 'a', 'ã': 'a', 'ạ': 'a', 'ă': 'a', 'ắ': 'a', 'ằ': 'a', 'ẳ': 'a', 'ẵ': 'a', 'ặ': 'a', 'â': 'a', 'ấ': 'a', 'ầ': 'a', 'ẩ': 'a', 'ẫ': 'a', 'ậ': 'a',
		'đ': 'd',
		'é': 'e', 'è': 'e', 'ẻ': 'e', 'ẽ': 'e', 'ẹ': 'e', 'ê': 'e', 'ế': 'e', 'ề': 'e', 'ể': 'e', 'ễ': 'e', 'ệ': 'e',
		'í': 'i', 'ì': 'i', 'ỉ': 'i', 'ĩ': 'i', 'ị': 'i',
		'ó': 'o', 'ò': 'o', 'ỏ': 'o', 'õ': 'o', 'ọ': 'o', 'ô': 'o', 'ố': 'o', 'ồ': 'o', 'ổ': 'o', 'ỗ': 'o', 'ộ': 'o', 'ơ': 'o', 'ớ': 'o', 'ờ': 'o', 'ở': 'o', 'ỡ': 'o', 'ợ': 'o',
		'ú': 'u', 'ù': 'u', 'ủ': 'u', 'ũ': 'u', 'ụ': 'u', 'ư': 'u', 'ứ': 'u', 'ừ': 'u', 'ử': 'u', 'ữ': 'u', 'ự': 'u',
		'ý': 'y', 'ỳ': 'y', 'ỷ': 'y', 'ỹ': 'y', 'ỵ': 'y',
		// Uppercase
		'Á': 'A', 'À': 'A', 'Ả': 'A', 'Ã': 'A', 'Ạ': 'A', 'Ă': 'A', 'Ắ': 'A', 'Ằ': 'A', 'Ẳ': 'A', 'Ẵ': 'A', 'Ặ': 'A', 'Â': 'A', 'Ấ': 'A', 'Ầ': 'A', 'Ẩ': 'A', 'Ẫ': 'A', 'Ậ': 'A',
		'Đ': 'D',
		'É': 'E', 'È': 'E', 'Ẻ': 'E', 'Ẽ': 'E', 'Ẹ': 'E', 'Ê': 'E', 'Ế': 'E', 'Ề': 'E', 'Ể': 'E', 'Ễ': 'E', 'Ệ': 'E',
		'Í': 'I', 'Ì': 'I', 'Ỉ': 'I', 'Ĩ': 'I', 'Ị': 'I',
		'Ó': 'O', 'Ò': 'O', 'Ỏ': 'O', 'Õ': 'O', 'Ọ': 'O', 'Ô': 'O', 'Ố': 'O', 'Ồ': 'O', 'Ổ': 'O', 'Ỗ': 'O', 'Ộ': 'O', 'Ơ': 'O', 'Ớ': 'O', 'Ờ': 'O', 'Ở': 'O', 'Ỡ': 'O', 'Ợ': 'O',
		'Ú': 'U', 'Ù': 'U', 'Ủ': 'U', 'Ũ': 'U', 'Ụ': 'U', 'Ư': 'U', 'Ứ': 'U', 'Ừ': 'U', 'Ử': 'U', 'Ữ': 'U', 'Ự': 'U',
		'Ý': 'Y', 'Ỳ': 'Y', 'Ỷ': 'Y', 'Ỹ': 'Y', 'Ỵ': 'Y',
	}

	var sb strings.Builder
	for _, r := range s {
		if val, ok := accents[r]; ok {
			sb.WriteRune(val)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func GenerateRelaxedRegex(s string) string {
	s = strings.ToLower(s)
	// Replace vowels with character classes
	res := ""
	for _, r := range s {
		switch r {
		case 'a':
			res += "[aáàảãạăắằẳẵặâấầẩẫậ]"
		case 'd':
			res += "[dđ]"
		case 'e':
			res += "[eéèẻẽẹêếềểễệ]"
		case 'i':
			res += "[iíìỉĩị]"
		case 'o':
			res += "[oóòỏõọôốồổỗộơớờởỡợ]"
		case 'u':
			res += "[uúùủũụưứừửữự]"
		case 'y':
			res += "[yýỳỷỹỵ]"
		default:
			// Escape special regex chars if any (basic ones)
			if strings.ContainsRune(".+*?^$()[]{}|\\", r) {
				res += "\\" + string(r)
			} else {
				res += string(r)
			}
		}
	}
	return res
}

// ToValidUTF8 cleans strings to ensure they are valid UTF-8
func ToValidUTF8(s string) string {
	return strings.ToValidUTF8(s, "")
}
