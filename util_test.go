package restconf

import (
	"bytes"
	"errors"
	"testing"

	"net/http"
	"net/url"

	"github.com/freeconf/yang/fc"
)

func Test_SplitAddress(t *testing.T) {
	tests := []struct {
		url     string
		address string
		port    string
		module  string
		path    string
		hasErr  bool
	}{
		{
			url:     "http://server:port/restconf/data/module:path/some=x/where",
			address: "http://server:port/restconf/data/",
			module:  "module",
			path:    "path/some=x/where",
		},
		{
			url:     "http://server/restconf=100/streams/module:path=z?p=1&z=x",
			address: "http://server/restconf=100/streams/",
			module:  "module",
			path:    "path=z?p=1&z=x",
		},
		{
			url:    "no-protocol",
			hasErr: true,
		},
		{
			url:    "foo://no-module-or-path",
			hasErr: true,
		},
		{
			url:    "foo://server/no-mount",
			hasErr: true,
		},
		{
			url:    "foo://server/mount/no-module",
			hasErr: true,
		},
		{
			url:     "foo://server/mount/module:",
			address: "foo://server/mount/",
			module:  "module",
			path:    "",
		},
	}
	for _, test := range tests {
		address, module, path, err := SplitAddress(test.url)
		if test.hasErr && err == nil {
			t.Error("Expected parse error ", test.url)
			continue
		}
		if !test.hasErr && err != nil {
			t.Error(err)
			continue
		}
		fc.AssertEqual(t, test.address, address)
		fc.AssertEqual(t, test.module, module)
		fc.AssertEqual(t, test.path, path)
	}
}

func Test_AppendUrlSegment(t *testing.T) {
	tests := [][]string{
		{
			"a", "b", "a/b",
		},
		{
			"a/", "b", "a/b",
		},
		{
			"a/", "/b", "a/b",
		},
		{
			"a", "/b", "a/b",
		},
		{
			"", "", "",
		},
		{
			"a/", "", "a/",
		},
		{
			"", "/b", "/b",
		},
	}
	for _, test := range tests {
		actual := appendUrlSegment(test[0], test[1])
		fc.AssertEqual(t, test[2], actual)
	}
}

func Test_ipAddrSplitHostPort(t *testing.T) {
	tests := [][]string{
		{"127.0.0.1:1000", "127.0.0.1", "1000"},
		{"127.0.0.1", "127.0.0.1", ""},
		{"[::1]:1000", "[::1]", "1000"},
		{"::1", "::1", ""},
		{"[0:0:0:0:0:0:0]:1000", "[0:0:0:0:0:0:0]", "1000"},
	}
	for _, test := range tests {
		host, port := ipAddrSplitHostPort(test[0])
		fc.AssertEqual(t, test[1], host)
		fc.AssertEqual(t, test[2], port)
	}
}

func TestDecodeErrorPath(t *testing.T) {
	fc.AssertEqual(t, "foo:some/path", decodeErrorPath("/restconf/data/foo:some/path"))
	fc.AssertEqual(t, "bartend:", decodeErrorPath("/restconf/data/bartend:"))
}

func Test_shift(t *testing.T) {
	tests := []struct {
		in              string
		expectedSegment string
		expectedPath    string
		expectedRaw     string
	}{
		{
			in:              "http://server:999/some/path/here",
			expectedSegment: "some",
			expectedPath:    "path/here",
		},
		{
			in:              "http://server:999/some/path/here?p=1&z=x",
			expectedSegment: "some",
			expectedPath:    "path/here",
		},
		{
			in:              "http://server:999/some/path=xxx%30xxx/here",
			expectedSegment: "some",
			expectedPath:    "path=xxx0xxx/here",
			expectedRaw:     "path=xxx%30xxx/here",
		},
		{
			in:              "some/path/here",
			expectedSegment: "some",
			expectedPath:    "path/here",
		},
		{
			in:              "some",
			expectedSegment: "some",
			expectedPath:    "",
		},
		{
			in:              "some/",
			expectedSegment: "some",
			expectedPath:    "",
		},
	}
	for _, test := range tests {
		t.Log(test.in)
		orig, err := url.Parse(test.in)
		if err != nil {
			panic(err)
		}
		actualSeg, actualPath := shift(orig, '/')
		fc.AssertEqual(t, test.expectedSegment, actualSeg)
		fc.AssertEqual(t, test.expectedPath, actualPath.Path)
		if test.expectedRaw != "" {
			fc.AssertEqual(t, test.expectedRaw, actualPath.RawPath)
		}
	}
}

func Test_shiftOptionalParamWithinSegment(t *testing.T) {
	tests := []struct {
		in    string
		seg   string
		param string
		path  string
	}{
		{
			in:   "http://server:999/some/path/here",
			seg:  "some",
			path: "path/here",
		},
		{
			in:  "some/",
			seg: "some",
		},
		{
			in:  "some=/",
			seg: "some",
		},
		{
			in:    "some=x/",
			param: "x",
			seg:   "some",
		},
		// NOTE: you cannot use following url encoded values as they match delims and
		// unescaped path parsing will not work.  details in code implementation
		//   %2f   /
		//   %3d   =
		{
			in:    "some=x%3ax/path",
			param: "x:x",
			seg:   "some",
			path:  "path",
		},
		{
			in:   "data/call-home-register:",
			seg:  "data",
			path: "call-home-register:",
		},
		{
			in:   "/some",
			seg:  "some",
			path: "",
		},
	}
	for _, test := range tests {
		orig, err := url.Parse(test.in)
		if err != nil {
			panic(err)
		}
		seg, param, path := shiftOptionalParamWithinSegment(orig, '=', '/')
		fc.AssertEqual(t, test.seg, seg)
		fc.AssertEqual(t, test.param, param)
		fc.AssertEqual(t, test.path, path.Path)
	}
}

func TestHandleErr(t *testing.T) {
	werr := errors.New("some error")
	r := http.Request{}
	w := dummyResponseWriter{}
	handleErr(Strict, werr, &r, &w, YangDataXmlMimeType1)
	fc.Gold(t, *updateFlag, w.buf.Bytes(), "testdata/gold/error.xml")

	w.buf.Reset()
	handleErr(Strict, werr, &r, &w, YangDataJsonMimeType1)
	fc.Gold(t, *updateFlag, w.buf.Bytes(), "testdata/gold/error.json")
}

type dummyResponseWriter struct {
	buf bytes.Buffer
}

// Write implements http.ResponseWriter.
func (e *dummyResponseWriter) Write(data []byte) (int, error) {
	return e.buf.Write(data)
}

// WriteHeader implements http.ResponseWriter.
func (dummyResponseWriter) WriteHeader(statusCode int) {
}

func (d dummyResponseWriter) Header() http.Header {
	return http.Header{}
}
