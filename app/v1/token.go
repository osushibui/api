package v1

import (
	"crypto/md5"
	"database/sql"
	"fmt"

	"github.com/osuripple/api/common"
	"golang.org/x/crypto/bcrypt"
)

type tokenNewInData struct {
	// either username or userid must be given in the request.
	// if none is given, the request is trashed.
	Username    string `json:"username"`
	UserID      int    `json:"id"`
	Password    string `json:"password"`
	Privileges  int    `json:"privileges"`
	Description string `json:"description"`
}

type tokenNewResponse struct {
	common.ResponseBase
	Username   string `json:"username"`
	ID         int    `json:"id"`
	Privileges int    `json:"privileges"`
	Token      string `json:"token,omitempty"`
	Banned     bool   `json:"banned"`
}

// TokenNewPOST is the handler for POST /token/new.
func TokenNewPOST(md common.MethodData) common.CodeMessager {
	var r tokenNewResponse
	data := tokenNewInData{}
	err := md.RequestData.Unmarshal(&data)
	if err != nil {
		return ErrBadJSON
	}

	var miss []string
	if data.Username == "" && data.UserID == 0 {
		miss = append(miss, "username|id")
	}
	if data.Password == "" {
		miss = append(miss, "password")
	}
	if len(miss) != 0 {
		return ErrMissingField(miss...)
	}

	var q *sql.Row
	const base = "SELECT id, username, rank, password_md5, password_version, allowed FROM users "
	if data.UserID != 0 {
		q = md.DB.QueryRow(base+"WHERE id = ? LIMIT 1", data.UserID)
	} else {
		q = md.DB.QueryRow(base+"WHERE username = ? LIMIT 1", data.Username)
	}

	var (
		rank      int
		pw        string
		pwVersion int
		allowed   int
	)

	err = q.Scan(&r.ID, &r.Username, &rank, &pw, &pwVersion, &allowed)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "No user with that username/id was found.")
	case err != nil:
		md.Err(err)
		return Err500
	}

	if nFailedAttempts(r.ID) > 20 {
		return common.SimpleResponse(429, "You've made too many login attempts. Try again later.")
	}

	if pwVersion == 1 {
		return common.SimpleResponse(418, "That user still has a password in version 1. Unfortunately, in order for the API to check for the password to be OK, the user has to first log in through the website.")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(pw), []byte(fmt.Sprintf("%x", md5.Sum([]byte(data.Password))))); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			go addFailedAttempt(r.ID)
			return common.SimpleResponse(403, "That password doesn't match!")
		}
		md.Err(err)
		return Err500
	}
	if allowed == 0 {
		r.Code = 200
		r.Message = "That user is banned."
		r.Banned = true
		return r
	}
	r.Privileges = int(common.Privileges(data.Privileges).CanOnly(rank))

	var (
		tokenStr string
		tokenMD5 string
	)
	for {
		tokenStr = common.RandomString(32)
		tokenMD5 = fmt.Sprintf("%x", md5.Sum([]byte(tokenStr)))
		r.Token = tokenStr
		id := 0

		err := md.DB.QueryRow("SELECT id FROM tokens WHERE token=? LIMIT 1", tokenMD5).Scan(&id)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			md.Err(err)
			return Err500
		}
	}
	_, err = md.DB.Exec("INSERT INTO tokens(user, privileges, description, token) VALUES (?, ?, ?, ?)", r.ID, r.Privileges, data.Description, tokenMD5)
	if err != nil {
		md.Err(err)
		return Err500
	}

	r.Code = 200
	return r
}
