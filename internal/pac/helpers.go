package pac

// The standard PAC helper functions (the Netscape PAC specification), written
// for this package in Go and registered on the JavaScript VM. They are original
// implementations, not copied from another PAC engine.

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/robertkrimen/otto"
)

// dnsTimeout bounds every DNS lookup a PAC script makes.
const dnsTimeout = 1 * time.Second

func register(vm *otto.Otto) error {
	funcs := map[string]func(otto.FunctionCall) otto.Value{
		"isPlainHostName":     isPlainHostName,
		"dnsDomainIs":         dnsDomainIs,
		"localHostOrDomainIs": localHostOrDomainIs,
		"isResolvable":        isResolvable,
		"isInNet":             isInNet,
		"dnsResolve":          dnsResolve,
		"myIpAddress":         myIPAddress,
		"dnsDomainLevels":     dnsDomainLevels,
		"shExpMatch":          shExpMatch,
		"weekdayRange":        weekdayRange,
		"dateRange":           alwaysTrue,
		"timeRange":           timeRange,
	}
	for name, f := range funcs {
		if err := vm.Set(name, f); err != nil {
			return err
		}
	}
	return nil
}

func arg(call otto.FunctionCall, i int) string {
	v := call.Argument(i)
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	s, _ := v.ToString()
	return s
}

func boolValue(b bool) otto.Value {
	v, _ := otto.ToValue(b)
	return v
}

func isPlainHostName(call otto.FunctionCall) otto.Value {
	return boolValue(!strings.Contains(arg(call, 0), "."))
}

func dnsDomainIs(call otto.FunctionCall) otto.Value {
	host, domain := strings.ToLower(arg(call, 0)), strings.ToLower(arg(call, 1))
	return boolValue(domain != "" && strings.HasSuffix(host, domain))
}

func localHostOrDomainIs(call otto.FunctionCall) otto.Value {
	host, hostdom := strings.ToLower(arg(call, 0)), strings.ToLower(arg(call, 1))
	if host == hostdom {
		return boolValue(true)
	}
	return boolValue(!strings.Contains(host, ".") && strings.HasPrefix(hostdom, host+"."))
}

func lookupIPv4(host string) string {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if v4 := a.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

func isResolvable(call otto.FunctionCall) otto.Value {
	return boolValue(lookupIPv4(arg(call, 0)) != "")
}

func dnsResolve(call otto.FunctionCall) otto.Value {
	ip := lookupIPv4(arg(call, 0))
	if ip == "" {
		return otto.NullValue()
	}
	v, _ := otto.ToValue(ip)
	return v
}

func myIPAddress(call otto.FunctionCall) otto.Value {
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := ifc.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if n, ok := a.(*net.IPNet); ok {
					if v4 := n.IP.To4(); v4 != nil {
						v, _ := otto.ToValue(v4.String())
						return v
					}
				}
			}
		}
	}
	v, _ := otto.ToValue("127.0.0.1")
	return v
}

func isInNet(call otto.FunctionCall) otto.Value {
	ip := net.ParseIP(lookupIPv4(arg(call, 0)))
	pattern := net.ParseIP(arg(call, 1))
	mask := net.ParseIP(arg(call, 2))
	if ip == nil || pattern == nil || mask == nil {
		return boolValue(false)
	}
	ip4, p4, m4 := ip.To4(), pattern.To4(), mask.To4()
	if ip4 == nil || p4 == nil || m4 == nil {
		return boolValue(false)
	}
	for i := 0; i < 4; i++ {
		if ip4[i]&m4[i] != p4[i]&m4[i] {
			return boolValue(false)
		}
	}
	return boolValue(true)
}

func dnsDomainLevels(call otto.FunctionCall) otto.Value {
	v, _ := otto.ToValue(strings.Count(arg(call, 0), "."))
	return v
}

// shExpMatch matches a shell expression: '*' is any run of characters and '?' is
// exactly one character. Unlike path.Match, '/' is not special.
func shExpMatch(call otto.FunctionCall) otto.Value {
	return boolValue(globMatch(arg(call, 1), arg(call, 0)))
}

func globMatch(pattern, s string) bool {
	p, n := 0, 0
	star, mark := -1, 0
	for n < len(s) {
		switch {
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == s[n]):
			p++
			n++
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, n
			p++
		case star >= 0:
			p = star + 1
			mark++
			n = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// now is replaceable in tests.
var now = time.Now

// clock returns the time to use and the arguments without a trailing "GMT".
func clock(call otto.FunctionCall) (time.Time, []string) {
	var args []string
	for i := range call.ArgumentList {
		args = append(args, arg(call, i))
	}
	t := now()
	if n := len(args); n > 0 && strings.EqualFold(args[n-1], "GMT") {
		args = args[:n-1]
		t = t.UTC()
	}
	return t, args
}

var weekdays = map[string]int{"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6}

// weekdayRange(wd1 [, wd2] [, "GMT"]): true on wd1, or between wd1 and wd2
// inclusive (wrapping, so "FRI","MON" covers the weekend).
func weekdayRange(call otto.FunctionCall) otto.Value {
	t, args := clock(call)
	if len(args) == 0 {
		return boolValue(false)
	}
	from, ok1 := weekdays[strings.ToUpper(args[0])]
	to, ok2 := from, true
	if len(args) > 1 {
		to, ok2 = weekdays[strings.ToUpper(args[1])]
	}
	if !ok1 || !ok2 {
		return boolValue(false)
	}
	d := int(t.Weekday())
	if from <= to {
		return boolValue(d >= from && d <= to)
	}
	return boolValue(d >= from || d <= to)
}

// timeRange(h) | (h1, h2) | (h1, m1, h2, m2) | (h1, m1, s1, h2, m2, s2), each with
// an optional trailing "GMT". Ranges may wrap past midnight.
func timeRange(call otto.FunctionCall) otto.Value {
	t, args := clock(call)
	n := make([]int, len(args))
	for i, a := range args {
		v, err := strconv.Atoi(a)
		if err != nil {
			return boolValue(false)
		}
		n[i] = v
	}
	cur := t.Hour()*3600 + t.Minute()*60 + t.Second()
	var from, to int
	switch len(n) {
	case 1:
		return boolValue(t.Hour() == n[0])
	case 2:
		from, to = n[0]*3600, n[1]*3600+3599
	case 4:
		from, to = n[0]*3600+n[1]*60, n[2]*3600+n[3]*60+59
	case 6:
		from, to = n[0]*3600+n[1]*60+n[2], n[3]*3600+n[4]*60+n[5]
	default:
		return boolValue(false)
	}
	if from <= to {
		return boolValue(cur >= from && cur <= to)
	}
	return boolValue(cur >= from || cur <= to)
}

// dateRange is not implemented: its many argument forms are rarely used in proxy
// routing. It returns true, so a rule that depends on it is applied rather than
// silently skipped.
func alwaysTrue(otto.FunctionCall) otto.Value { return boolValue(true) }

// itoa is used by tests.
func itoa(i int) string { return strconv.Itoa(i) }
