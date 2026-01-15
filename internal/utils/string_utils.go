package utils

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// SanitizeHTML strips HTML tags, script/style content, and decodes entities
func SanitizeHTML(s string) string {
	// 1. Decode HTML entities first (e.g. &lt; -> <) so tags are recognized
	s = html.UnescapeString(s)

	// 2. Remove script and style blocks content
	reScript := regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`)
	s = reScript.ReplaceAllString(s, "")
	reStyle := regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`)
	s = reStyle.ReplaceAllString(s, "")

	// 3. Strip tags using bluemonday
	p := bluemonday.StripTagsPolicy()
	s = p.Sanitize(s)

	// 4. Decode HTML entities AGAIN (bluemonday might have escaped them, and we want plain text)
	s = html.UnescapeString(s)

	// 5. Collapse extra whitespace
	s = strings.Join(strings.Fields(s), " ")

	return s
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

// ParseJSON parses a JSON string into a target interface
func ParseJSON(jsonStr string, target interface{}) error {
	return json.Unmarshal([]byte(jsonStr), target)
}
