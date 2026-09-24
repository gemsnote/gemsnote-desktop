package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gemsnote/gemsnote/db"
	"github.com/gemsnote/gemsnote/models"
	"github.com/gemsnote/gemsnote/service"
	"github.com/gemsnote/gemsnote/utils"
	"github.com/sirupsen/logrus"
)

// This task owns its immutable account credentials, paths and cancellation
// context. It must never reuse ServerProxy's currently logged-in account.
func RefreshAccountProfile(ctx context.Context, user models.User, database *db.Database, files *service.FileService) {
	if err := refreshAccountAvatar(ctx, user, database, files); err != nil && ctx.Err() == nil {
		logrus.Warnf("Background avatar refresh failed: %v", err)
	}
}

func refreshAccountAvatar(ctx context.Context, user models.User, database *db.Database, files *service.FileService) error {
	originalLogo, err := database.GetConfig("logo:" + user.ID)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	get := func(target string, token bool) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid avatar request URL")
		}
		if token {
			q := req.URL.Query()
			q.Set("token", user.Token)
			req.URL.RawQuery = q.Encode()
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("avatar request failed: %s", strings.ReplaceAll(err.Error(), user.Token, "[redacted]"))
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("avatar request returned HTTP %d", resp.StatusCode)
		}
		return resp, nil
	}
	resp, err := get(strings.TrimRight(user.Host, "/")+"/api2/user/info", true)
	if err != nil {
		return err
	}
	var profile struct {
		UserID string `json:"UserId"`
		Logo   string
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&profile)
	resp.Body.Close()
	if err != nil || profile.UserID != user.ID {
		return fmt.Errorf("invalid avatar profile")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	logo := strings.TrimSpace(profile.Logo)
	if logo == "" {
		return database.SetConfig("logo:"+user.ID, "")
	}
	base, err := url.Parse(strings.TrimRight(user.Host, "/") + "/")
	if err != nil {
		return err
	}
	ref, err := url.Parse(logo)
	if err != nil {
		return fmt.Errorf("invalid avatar URL")
	}
	target := base.ResolveReference(ref)
	if target.Scheme != base.Scheme || target.Host != base.Host {
		// Do not send authentication to a third party.
		if target.Scheme != "https" && target.Scheme != "http" {
			return fmt.Errorf("unsupported avatar URL")
		}
		return database.SetConfig("logo:"+user.ID, target.String())
	}
	resp, err = get(target.String(), strings.HasPrefix(target.Path, "/api2/"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		return fmt.Errorf("avatar response is not an image")
	}
	file, err := os.CreateTemp(files.GetUserImageDir(user.ID), "avatar-*")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			os.Remove(file.Name())
		}
	}()
	n, err := io.Copy(file, io.LimitReader(resp.Body, (10<<20)+1))
	if err != nil {
		return fmt.Errorf("avatar download interrupted")
	}
	if n > 10<<20 {
		return fmt.Errorf("avatar exceeds size limit")
	}
	if err = file.Close(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if current, err := database.GetConfig("logo:" + user.ID); err != nil || current != originalLogo {
		return err
	}
	id, now := utils.ObjectId(), time.Now()
	if err = database.InsertImage(&models.Image{ID: id, FileID: id, ServerFileID: id, UserID: user.ID, Path: file.Name(), CreatedTime: &now}); err != nil {
		return err
	}
	keep = true
	if err = database.SetConfig("logo_source:"+user.ID, logo); err != nil {
		return err
	}
	return database.SetConfig("logo:"+user.ID, "/api2/file/getImage?fileId="+id)
}
