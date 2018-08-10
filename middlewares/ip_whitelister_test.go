package middlewares

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containous/traefik/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/negroni"
)

func TestNewIPWhitelister(t *testing.T) {
	cases := []struct {
		desc               string
		whitelistStrings   []string
		expectedWhitelists []*net.IPNet
		errMessage         string
	}{
		{
			desc:               "nil whitelist",
			whitelistStrings:   nil,
			expectedWhitelists: nil,
			errMessage:         "no whitelists provided",
		}, {
			desc:               "empty whitelist",
			whitelistStrings:   []string{},
			expectedWhitelists: nil,
			errMessage:         "no whitelists provided",
		}, {
			desc: "whitelist containing empty string",
			whitelistStrings: []string{
				"1.2.3.4/24",
				"",
				"fe80::/16",
			},
			expectedWhitelists: nil,
			errMessage:         "parsing CIDR whitelist <nil>: invalid CIDR address: ",
		}, {
			desc: "whitelist containing only an empty string",
			whitelistStrings: []string{
				"",
			},
			expectedWhitelists: nil,
			errMessage:         "parsing CIDR whitelist <nil>: invalid CIDR address: ",
		}, {
			desc: "whitelist containing an invalid string",
			whitelistStrings: []string{
				"foo",
			},
			expectedWhitelists: nil,
			errMessage:         "parsing CIDR whitelist <nil>: invalid CIDR address: foo",
		}, {
			desc: "IPv4 & IPv6 whitelist",
			whitelistStrings: []string{
				"1.2.3.4/24",
				"fe80::/16",
			},
			expectedWhitelists: []*net.IPNet{
				{IP: net.IPv4(1, 2, 3, 0).To4(), Mask: net.IPv4Mask(255, 255, 255, 0)},
				{IP: net.ParseIP("fe80::"), Mask: net.IPMask(net.ParseIP("ffff::"))},
			},
			errMessage: "",
		}, {
			desc: "IPv4 only",
			whitelistStrings: []string{
				"127.0.0.1/8",
			},
			expectedWhitelists: []*net.IPNet{
				{IP: net.IPv4(127, 0, 0, 0).To4(), Mask: net.IPv4Mask(255, 0, 0, 0)},
			},
			errMessage: "",
		},
	}

	for _, test := range cases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			whitelister, err := NewIPWhitelister(test.whitelistStrings, false)
			if test.errMessage != "" {
				require.EqualError(t, err, test.errMessage)
			} else {
				require.NoError(t, err)
				for index, actual := range whitelister.whitelists {
					expected := test.expectedWhitelists[index]
					assert.Equal(t, expected.IP, actual.IP)
					assert.Equal(t, expected.Mask.String(), actual.Mask.String())
				}
			}
		})
	}
}

func TestIPWhitelisterHandle(t *testing.T) {
	cases := []struct {
		desc             string
		whitelistStrings []string
		passIPs          []string
		rejectIPs        []string
	}{
		{
			desc: "IPv4",
			whitelistStrings: []string{
				"1.2.3.4/24",
			},
			passIPs: []string{
				"1.2.3.1",
				"1.2.3.32",
				"1.2.3.156",
				"1.2.3.255",
			},
			rejectIPs: []string{
				"1.2.16.1",
				"1.2.32.1",
				"127.0.0.1",
				"8.8.8.8",
			},
		},
		{
			desc: "IPv4 single IP",
			whitelistStrings: []string{
				"8.8.8.8/32",
			},
			passIPs: []string{
				"8.8.8.8",
			},
			rejectIPs: []string{
				"8.8.8.7",
				"8.8.8.9",
				"8.8.8.0",
				"8.8.8.255",
				"4.4.4.4",
				"127.0.0.1",
			},
		},
		{
			desc: "multiple IPv4",
			whitelistStrings: []string{
				"1.2.3.4/24",
				"8.8.8.8/8",
			},
			passIPs: []string{
				"1.2.3.1",
				"1.2.3.32",
				"1.2.3.156",
				"1.2.3.255",
				"8.8.4.4",
				"8.0.0.1",
				"8.32.42.128",
				"8.255.255.255",
			},
			rejectIPs: []string{
				"1.2.16.1",
				"1.2.32.1",
				"127.0.0.1",
				"4.4.4.4",
				"4.8.8.8",
			},
		},
		{
			desc: "IPv6",
			whitelistStrings: []string{
				"2a03:4000:6:d080::/64",
			},
			passIPs: []string{
				"[2a03:4000:6:d080::]",
				"[2a03:4000:6:d080::1]",
				"[2a03:4000:6:d080:dead:beef:ffff:ffff]",
				"[2a03:4000:6:d080::42]",
			},
			rejectIPs: []string{
				"[2a03:4000:7:d080::]",
				"[2a03:4000:7:d080::1]",
				"[fe80::]",
				"[4242::1]",
			},
		},
		{
			desc: "IPv6 single IP",
			whitelistStrings: []string{
				"2a03:4000:6:d080::42/128",
			},
			passIPs: []string{
				"[2a03:4000:6:d080::42]",
			},
			rejectIPs: []string{
				"[2a03:4000:6:d080::1]",
				"[2a03:4000:6:d080:dead:beef:ffff:ffff]",
				"[2a03:4000:6:d080::43]",
			},
		},
		{
			desc: "multiple IPv6",
			whitelistStrings: []string{
				"2a03:4000:6:d080::/64",
				"fe80::/16",
			},
			passIPs: []string{
				"[2a03:4000:6:d080::]",
				"[2a03:4000:6:d080::1]",
				"[2a03:4000:6:d080:dead:beef:ffff:ffff]",
				"[2a03:4000:6:d080::42]",
				"[fe80::1]",
				"[fe80:aa00:00bb:4232:ff00:eeee:00ff:1111]",
				"[fe80::fe80]",
			},
			rejectIPs: []string{
				"[2a03:4000:7:d080::]",
				"[2a03:4000:7:d080::1]",
				"[4242::1]",
			},
		},
		{
			desc: "multiple IPv6 & IPv4",
			whitelistStrings: []string{
				"2a03:4000:6:d080::/64",
				"fe80::/16",
				"1.2.3.4/24",
				"8.8.8.8/8",
			},
			passIPs: []string{
				"[2a03:4000:6:d080::]",
				"[2a03:4000:6:d080::1]",
				"[2a03:4000:6:d080:dead:beef:ffff:ffff]",
				"[2a03:4000:6:d080::42]",
				"[fe80::1]",
				"[fe80:aa00:00bb:4232:ff00:eeee:00ff:1111]",
				"[fe80::fe80]",
				"1.2.3.1",
				"1.2.3.32",
				"1.2.3.156",
				"1.2.3.255",
				"8.8.4.4",
				"8.0.0.1",
				"8.32.42.128",
				"8.255.255.255",
			},
			rejectIPs: []string{
				"[2a03:4000:7:d080::]",
				"[2a03:4000:7:d080::1]",
				"[4242::1]",
				"1.2.16.1",
				"1.2.32.1",
				"127.0.0.1",
				"4.4.4.4",
				"4.8.8.8",
			},
		},
		{
			desc: "broken IP-addresses",
			whitelistStrings: []string{
				"127.0.0.1/32",
			},
			passIPs: nil,
			rejectIPs: []string{
				"foo",
				"10.0.0.350",
				"fe:::80",
				"",
				"\\&$§&/(",
			},
		},
	}

	for _, test := range cases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()
			whitelisterNoHeaderCheck, err := NewIPWhitelister(test.whitelistStrings, false)
			require.NoError(t, err)
			require.NotNil(t, whitelisterNoHeaderCheck)

			whitelisterHeaderCheck, err := NewIPWhitelister(test.whitelistStrings, true)
			require.NoError(t, err)
			require.NotNil(t, whitelisterHeaderCheck)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, "traefik")
			})
			n := negroni.New(whitelisterNoHeaderCheck)
			n.UseHandler(handler)

			n2 := negroni.New(whitelisterHeaderCheck)
			n2.UseHandler(handler)

			// assert whitelisted IPs in remoteAddr pass.
			for _, testIP := range test.passIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)

				req.RemoteAddr = testIP + ":2342"
				recorder := httptest.NewRecorder()
				n.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusOK, recorder.Code, testIP+" should have passed "+test.desc)
				assert.Contains(t, recorder.Body.String(), "traefik")
			}

			// assert non-whitelisted IPs in remoteAddr fail.
			for _, testIP := range test.rejectIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)

				req.RemoteAddr = testIP + ":2342"
				recorder := httptest.NewRecorder()
				n.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusForbidden, recorder.Code, testIP+" should not have passed "+test.desc)
				assert.NotContains(t, recorder.Body.String(), "traefik")
			}

			// assert valid IPs in X-Forwarded-For, pass when whitelistCheckHeaders = true (n2).
			for _, testIP := range test.passIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "254.254.254.254:2342"
				req.Header.Set("X-Forwarded-For", strings.Trim(testIP, "[]"))
				recorder := httptest.NewRecorder()
				n2.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusOK, recorder.Code, testIP+" should have passed "+test.desc)
				assert.Contains(t, recorder.Body.String(), "traefik")
			}

			// assert valid IPs in X-Forwarded-For, fail when whitelistCheckHeaders = false (n).
			for _, testIP := range test.passIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "254.254.254.254:2342"
				req.Header.Set("X-Forwarded-For", strings.Trim(testIP, "[]"))
				recorder := httptest.NewRecorder()
				n.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusForbidden, recorder.Code, testIP+" should not have passed "+test.desc)
				assert.NotContains(t, recorder.Body.String(), "traefik")
			}

			// assert invalid IPs in X-Forwarded-For, fail when whitelistCheckHeaders = true (n2).
			for _, testIP := range test.rejectIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "254.254.254.254:2342"
				req.Header.Set("X-Forwarded-For", strings.Trim(testIP, "[]"))
				recorder := httptest.NewRecorder()
				n2.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusForbidden, recorder.Code, testIP+" should not have passed "+test.desc)
				assert.NotContains(t, recorder.Body.String(), "traefik")
			}

			// assert valid IPs in X-Real-Ip, pass when whitelistCheckHeaders = true (n2).
			for _, testIP := range test.passIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "254.254.254.254:2342"
				req.Header.Set("X-Real-Ip", strings.Trim(testIP, "[]"))
				recorder := httptest.NewRecorder()
				n2.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusOK, recorder.Code, testIP+" should have passed "+test.desc)
				assert.Contains(t, recorder.Body.String(), "traefik")
			}
		})
	}
}

func TestIPWhitelisterHandleMultiIp(t *testing.T) {
	cases := []struct {
		desc             string
		whitelistStrings []string
		passIPs          []string
		rejectIPs        []string
	}{
		{
			desc: "Only looks at last IP in X-Forwarded-For",
			whitelistStrings: []string{
				"1.2.3.4/32",
			},
			passIPs: []string{
				"1.1.1.1,1.2.3.4",
				"1.1.1.1,2.2.2.2,1.2.3.4",
				"1.2.3.4",
			},
			rejectIPs: []string{
				"1.2.3.4,1.1.1.1",
				"1.1.1.1,1.2.3.4,2.2.2.2",
				"1.1.1.1",
			},
		},
	}

	for _, test := range cases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			whitelisterHeaderCheck, err := NewIPWhitelister(test.whitelistStrings, true)
			require.NoError(t, err)
			require.NotNil(t, whitelisterHeaderCheck)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, "traefik")
			})

			n2 := negroni.New(whitelisterHeaderCheck)
			n2.UseHandler(handler)

			// assert valid IPs in X-Forwarded-For, fail when whitelistCheckHeaders = false (n).
			for _, testIP := range test.passIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "254.254.254.254:2342"
				req.Header.Set("X-Forwarded-For", strings.Trim(testIP, "[]"))
				recorder := httptest.NewRecorder()
				n2.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusOK, recorder.Code, testIP+" should have passed "+test.desc)
				assert.Contains(t, recorder.Body.String(), "traefik")
			}

			// assert invalid IPs in X-Forwarded-For, fail when whitelistCheckHeaders = true (n2).
			for _, testIP := range test.rejectIPs {
				req := testhelpers.MustNewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "254.254.254.254:2342"
				req.Header.Set("X-Forwarded-For", strings.Trim(testIP, "[]"))
				recorder := httptest.NewRecorder()
				n2.ServeHTTP(recorder, req)

				assert.Equal(t, http.StatusForbidden, recorder.Code, testIP+" should not have passed "+test.desc)
				assert.NotContains(t, recorder.Body.String(), "traefik")
			}
		})
	}
}
