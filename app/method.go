package app

import (
	"encoding/json"
	"fmt"
	"regexp"
	"unsafe"

	"github.com/valyala/fasthttp"
	"zxq.co/ripple/rippleapi/common"
)

// Method wraps an API method to a HandlerFunc.
func Method(f func(md common.MethodData) common.CodeMessager, privilegesNeeded ...int) fasthttp.RequestHandler {
	return func(c *fasthttp.RequestCtx) {
		initialCaretaker(c, f, privilegesNeeded...)
	}
}

func initialCaretaker(c *fasthttp.RequestCtx, f func(md common.MethodData) common.CodeMessager, privilegesNeeded ...int) {
	var doggoTags []string

	qa := c.Request.URI().QueryArgs()
	var token string
	switch {
	case len(c.Request.Header.Peek("X-Ripple-Token")) > 0:
		token = string(c.Request.Header.Peek("X-Ripple-Token"))
	case len(qa.Peek("token")) > 0:
		token = string(qa.Peek("token"))
	case len(qa.Peek("k")) > 0:
		token = string(qa.Peek("k"))
	default:
		token = string(c.Request.Header.Cookie("rt"))
	}

	md := common.MethodData{
		DB:    db,
		Ctx:   c,
		Doggo: doggo,
		R:     red,
	}
	if token != "" {
		tokenReal, exists := GetTokenFull(token, db)
		if exists {
			md.User = tokenReal
			doggoTags = append(doggoTags, "authorised")
		}
	}

	// log into datadog that this is an hanayo request
	if b2s(c.Request.Header.Peek("H-Key")) == cf.HanayoKey && b2s(c.UserAgent()) == "hanayo" {
		doggoTags = append(doggoTags, "hanayo")
	}

	doggo.Incr("requests.v1", doggoTags, 1)

	missingPrivileges := 0
	for _, privilege := range privilegesNeeded {
		if uint64(md.User.TokenPrivileges)&uint64(privilege) == 0 {
			missingPrivileges |= privilege
		}
	}
	if missingPrivileges != 0 {
		c.SetStatusCode(401)
		mkjson(c, common.SimpleResponse(401, "You don't have the privilege(s): "+common.Privileges(missingPrivileges).String()+"."))
		return
	}

	resp := f(md)
	if md.HasQuery("pls200") {
		c.SetStatusCode(200)
	} else {
		c.SetStatusCode(resp.GetCode())
	}

	if md.HasQuery("callback") {
		c.Response.Header.Add("Content-Type", "application/javascript; charset=utf-8")
	} else {
		c.Response.Header.Add("Content-Type", "application/json; charset=utf-8")
	}

	mkjson(c, resp)
}

// Very restrictive, but this way it shouldn't completely fuck up.
var callbackJSONP = regexp.MustCompile(`^[a-zA-Z_\$][a-zA-Z0-9_\$]*$`)

// mkjson auto indents json, and wraps json into a jsonp callback if specified by the request.
// then writes to the RequestCtx the data.
func mkjson(c *fasthttp.RequestCtx, data interface{}) {
	exported, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		fmt.Println(err)
		exported = []byte(`{ "code": 500, "message": "something has gone really really really really really really wrong." }`)
	}
	cb := string(c.URI().QueryArgs().Peek("callback"))
	willcb := cb != "" &&
		len(cb) < 100 &&
		callbackJSONP.MatchString(cb)
	if willcb {
		c.Write([]byte("/**/ typeof " + cb + " === 'function' && " + cb + "("))
	}
	c.Write(exported)
	if willcb {
		c.Write([]byte(");"))
	}
}

// b2s converts byte slice to a string without memory allocation.
// See https://groups.google.com/forum/#!msg/Golang-Nuts/ENgbUzYvCuU/90yGx7GUAgAJ .
//
// Note it may break if string and/or slice header will change
// in the future go versions.
func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
