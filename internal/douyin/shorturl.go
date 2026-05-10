package douyin

import "regexp"

var shortURLRe = regexp.MustCompile(`https?://v\.douyin\.com/[A-Za-z0-9]+/?`)

// ExtractShortURL finds the first v.douyin.com short URL in input.
// Returns ErrNoShortURL if none is present.
func ExtractShortURL(input string) (string, error) {
	match := shortURLRe.FindString(input)
	if match == "" {
		return "", ErrNoShortURL
	}
	return match, nil
}
