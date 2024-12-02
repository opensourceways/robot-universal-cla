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
	"flag"
	"github.com/opensourceways/robot-framework-lib/config"
	"github.com/opensourceways/server-common-lib/secret"
	"github.com/sirupsen/logrus"
	"os"
)

type robotOptions struct {
	service   config.FrameworkOptions
	delToken  bool
	interrupt bool
	tokenPath string
}

func (o *robotOptions) addFlags(fs *flag.FlagSet) {
	o.service.AddFlagsComposite(fs)
	fs.StringVar(
		&o.tokenPath, "token-path", "",
		"Path to the file containing the token secret.",
	)
	fs.BoolVar(
		&o.delToken, "del-token", true,
		"An flag to delete token secret file.",
	)
}

func (o *robotOptions) validateFlags() (*configuration, []byte) {
	if err := o.service.ValidateComposite(); err != nil {
		logrus.WithError(err).Errorf("invalid service options")
		o.interrupt = true
		return nil, nil
	}

	configmap, err := config.NewConfigmapAgent(&configuration{}, o.service.ConfigFile)
	if err != nil {
		logrus.WithError(err).Error("fatal error occurred while loading and parsing configmap")
		o.interrupt = true
		return nil, nil
	}

	token, err := secret.LoadSingleSecret(o.tokenPath)
	if err != nil {
		logrus.WithError(err).Error("fatal error occurred while loading token")
		o.interrupt = true
	}
	if o.delToken {
		if err = os.Remove(o.tokenPath); err != nil {
			logrus.WithError(err).Error("fatal error occurred while deleting token")
			o.interrupt = true
		}
	}

	return configmap.GetConfigmap().(*configuration), token
}

// gatherOptions gather the necessary arguments from command line for project startup.
// It returns the configuration and the token to using for subsequent processes.
func (o *robotOptions) gatherOptions(fs *flag.FlagSet, args ...string) (*configuration, []byte) {
	o.addFlags(fs)
	_ = fs.Parse(args)
	cnf, token := o.validateFlags()

	return cnf, token
}
