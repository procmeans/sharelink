package douyin

import "regexp"

var shortURLRe = regexp.MustCompile(`https?://v\.douyin\.com/[A-Za-z0-9_-]+/?`)

// Matches reports whether input contains a v.douyin.com short URL.
func Matches(input string) bool { return shortURLRe.MatchString(input) }

// ExtractShortURL finds the first v.douyin.com short URL in input.
// Returns ErrNoShortURL if none is present.
func ExtractShortURL(input string) (string, error) {
	match := shortURLRe.FindString(input)
	if match == "" {
		return "", ErrNoShortURL
	}
	return match, nil
}
