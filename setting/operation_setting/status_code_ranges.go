package operation_setting

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

type StatusCodeRange struct {
	Start int
	End   int
}

// RetryStatusCodeBadResponseBody is a pseudo status code used by the retry
// policy when the upstream response body cannot be parsed into a usable
// response. It is rendered as 000 in the operation setting.
const RetryStatusCodeBadResponseBody = 0

var AutomaticDisableStatusCodeRanges = []StatusCodeRange{{Start: 401, End: 401}}

// The HTTP defaults preserve the previous behavior while 000 enables retries
// when a response body cannot yield billable usage.
var AutomaticRetryStatusCodeRanges = []StatusCodeRange{
	{Start: RetryStatusCodeBadResponseBody, End: RetryStatusCodeBadResponseBody},
	{Start: 100, End: 199},
	{Start: 300, End: 399},
	{Start: 401, End: 407},
	{Start: 409, End: 499},
	{Start: 500, End: 503},
	{Start: 505, End: 523},
	{Start: 525, End: 599},
}

func AutomaticDisableStatusCodesToString() string {
	return statusCodeRangesToString(AutomaticDisableStatusCodeRanges)
}

func AutomaticDisableStatusCodesFromString(s string) error {
	ranges, err := ParseHTTPStatusCodeRanges(s)
	if err != nil {
		return err
	}
	AutomaticDisableStatusCodeRanges = ranges
	return nil
}

func ShouldDisableByStatusCode(code int) bool {
	return shouldMatchStatusCodeRanges(AutomaticDisableStatusCodeRanges, code)
}

func AutomaticRetryStatusCodesToString() string {
	return statusCodeRangesToString(AutomaticRetryStatusCodeRanges)
}

func AutomaticRetryStatusCodesFromString(s string) error {
	ranges, err := ParseRetryStatusCodeRanges(s)
	if err != nil {
		return err
	}
	AutomaticRetryStatusCodeRanges = ranges
	return nil
}

func ShouldRetryByStatusCode(code int) bool {
	return shouldMatchStatusCodeRanges(AutomaticRetryStatusCodeRanges, code)
}

// RetryStatusCodeForError maps internal response parsing errors to the
// configurable pseudo status code. Real HTTP status codes remain unchanged.
func RetryStatusCodeForError(errorCode types.ErrorCode, statusCode int) int {
	if errorCode == types.ErrorCodeBadResponseBody {
		return RetryStatusCodeBadResponseBody
	}
	return statusCode
}

// ShouldRetryByError applies the configured retry status rules to an API
// error, including the 000 mapping for ErrorCodeBadResponseBody.
func ShouldRetryByError(errorCode types.ErrorCode, statusCode int) bool {
	return ShouldRetryByStatusCode(RetryStatusCodeForError(errorCode, statusCode))
}

func statusCodeRangesToString(ranges []StatusCodeRange) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.Start == r.End {
			if r.Start == RetryStatusCodeBadResponseBody {
				parts = append(parts, "000")
				continue
			}
			parts = append(parts, strconv.Itoa(r.Start))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
	}
	return strings.Join(parts, ",")
}

func shouldMatchStatusCodeRanges(ranges []StatusCodeRange, code int) bool {
	if code != RetryStatusCodeBadResponseBody && (code < 100 || code > 599) {
		return false
	}
	for _, r := range ranges {
		if code < r.Start {
			return false
		}
		if code <= r.End {
			return true
		}
	}
	return false
}

func ParseHTTPStatusCodeRanges(input string) ([]StatusCodeRange, error) {
	return parseStatusCodeRanges(input, false)
}

func ParseRetryStatusCodeRanges(input string) ([]StatusCodeRange, error) {
	return parseStatusCodeRanges(input, true)
}

func parseStatusCodeRanges(input string, allowBadResponseBody bool) ([]StatusCodeRange, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	input = strings.NewReplacer("，", ",").Replace(input)
	segments := strings.Split(input, ",")

	var ranges []StatusCodeRange
	var invalid []string

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		r, err := parseHTTPStatusCodeToken(seg, allowBadResponseBody)
		if err != nil {
			invalid = append(invalid, seg)
			continue
		}
		ranges = append(ranges, r)
	}

	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid http status code rules: %s", strings.Join(invalid, ", "))
	}
	if len(ranges) == 0 {
		return nil, nil
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})

	merged := []StatusCodeRange{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.Start <= last.End+1 {
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		merged = append(merged, r)
	}

	return merged, nil
}

func parseHTTPStatusCodeToken(token string, allowBadResponseBody bool) (StatusCodeRange, error) {
	token = strings.TrimSpace(token)
	token = strings.ReplaceAll(token, " ", "")
	if token == "" {
		return StatusCodeRange{}, fmt.Errorf("empty token")
	}

	if strings.Contains(token, "-") {
		parts := strings.Split(token, "-")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return StatusCodeRange{}, fmt.Errorf("invalid range token: %s", token)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return StatusCodeRange{}, fmt.Errorf("invalid range start: %s", token)
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return StatusCodeRange{}, fmt.Errorf("invalid range end: %s", token)
		}
		if start > end {
			return StatusCodeRange{}, fmt.Errorf("range start > end: %s", token)
		}
		if !isValidStatusCode(start, allowBadResponseBody) || !isValidStatusCode(end, allowBadResponseBody) {
			return StatusCodeRange{}, fmt.Errorf("range out of bounds: %s", token)
		}
		if (start == RetryStatusCodeBadResponseBody) != (end == RetryStatusCodeBadResponseBody) {
			return StatusCodeRange{}, fmt.Errorf("range out of bounds: %s", token)
		}
		return StatusCodeRange{Start: start, End: end}, nil
	}

	code, err := strconv.Atoi(token)
	if err != nil {
		return StatusCodeRange{}, fmt.Errorf("invalid status code: %s", token)
	}
	if !isValidStatusCode(code, allowBadResponseBody) {
		return StatusCodeRange{}, fmt.Errorf("status code out of bounds: %s", token)
	}
	return StatusCodeRange{Start: code, End: code}, nil
}

func isValidStatusCode(code int, allowBadResponseBody bool) bool {
	return (allowBadResponseBody && code == RetryStatusCodeBadResponseBody) || (code >= 100 && code <= 599)
}
