// Copyright (c) Huawei Technologies Co., Ltd. 2024. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"errors"
	"github.com/opensourceways/server-common-lib/config"
	"reflect"
	"strings"
)

// configuration holds a list of repoConfig configurations.
type configuration struct {
	ConfigItems                  []repoConfig `json:"config_items,omitempty"`
	UserMarkFormat               string       `json:"user_mark_format" required:"true"`
	CommentCommandTrigger        string       `json:"comment_command_trigger" required:"true"`
	CommentPRNoCommits           string       `json:"comment_pr_no_commits" required:"true"`
	CommentAllSigned             string       `json:"comment_all_signed" required:"true"`
	CommentSomeNeedSign          string       `json:"comment_some_need_sign" required:"true"`
	CommentUpdateLabelFailed     string       `json:"comment_update_label_failed" required:"true"`
	PlaceholderCommitter         string       `json:"placeholder_committer" required:"true"`
	PlaceholderCLASignGuideTitle string       `json:"placeholder_cla_sign_guide_title" required:"true"`
	PlaceholderCLASignPassTitle  string       `json:"placeholder_cla_sign_pass_title" required:"true"`
}

// Validate to check the configmap data's validation, returns an error if invalid
func (c *configuration) Validate() error {
	if c == nil {
		return errors.New("configuration is nil")
	}

	// Validate each repo configuration
	items := c.ConfigItems
	for i := range items {
		if err := items[i].validateRepoConfig(); err != nil {
			return err
		}
	}

	return validateRequiredConfig(*c)
}

func validateRequiredConfig[C configuration | repoConfig](c C) error {
	k := reflect.TypeOf(c)
	v := reflect.ValueOf(c)

	var missing []string
	n := k.NumField()
	for i := 0; i < n; i++ {
		tag := k.Field(i).Tag.Get("required")
		if len(tag) > 0 {
			s, _ := v.Field(i).Interface().(string)
			if s == "" {
				missing = append(missing, k.Field(i).Tag.Get("json"))
			}
		}
	}

	if len(missing) != 0 {
		return errors.New("missing the follow config: " + strings.Join(missing, ", "))
	}

	return nil
}

// getRepoConfig retrieves a repoConfig for a given organization and repository.
// Returns the repoConfig if found, otherwise returns nil.
func (c *configuration) getRepoConfig(org, repo string) *repoConfig {
	if c == nil || len(c.ConfigItems) == 0 {
		return nil
	}

	for i := range c.ConfigItems {
		ok, _ := c.ConfigItems[i].RepoFilter.CanApply(org, org+"/"+repo)
		if ok {
			return &c.ConfigItems[i]
		}
	}

	return nil
}

// repoConfig is a configuration struct for a organization and repository.
// It includes a RepoFilter and a boolean value indicating if an issue can be closed only when its linking PR exists.
type repoConfig struct {
	// RepoFilter is used to filter repositories.
	config.RepoFilter
	// CLALabelYes is the cla label name for org/repos indicating
	// the cla has been signed
	CLALabelYes string `json:"cla_label_yes" required:"true"`

	// CLALabelNo is the cla label name for org/repos indicating
	// the cla has not been signed
	CLALabelNo string `json:"cla_label_no" required:"true"`

	// CheckURL is the url used to check whether the contributor has signed cla
	// The url has the format as https://**/{{org}}:{{repo}}?email={{email}}
	CheckURL string `json:"check_url" required:"true"`

	// SignURL is the url used to sign the cla
	SignURL string `json:"sign_url" required:"true"`

	// CheckByCommitter is one of ways to check CLA. There are two ways to check cla.
	// One is checking CLA by the email of committer, and Second is by the email of author.
	// Default is by email of author.
	CheckByCommitter bool `json:"check_by_committer"`

	// LitePRCommitter is the config for lite pr committer.
	// It must be set when `check_by_committer` is true.
	LitePRCommitter litePRCommiter `json:"lite_pr_committer"`

	// FAQURL is the url of faq which is corresponding to the way of checking CLA
	FAQURL string `json:"faq_url" required:"true"`
}

// validateRepoConfig to check the repoConfig data's validation, returns an error if invalid
func (c *repoConfig) validateRepoConfig() error {
	// If the bot is not configured to monitor any repositories, return an error.
	if len(c.Repos) == 0 {
		return errors.New("the repositories configuration can not be empty")
	}

	if err := c.RepoFilter.Validate(); err != nil {
		return err
	}

	return validateRequiredConfig(*c)
}

type litePRCommiter struct {
	// Email is the one of committer in a commit when a PR is lite
	Email string `json:"email" required:"true"`

	// Name is the one of committer in a commit when a PR is lite
	Name string `json:"name" required:"true"`
}
