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
	"github.com/opensourceways/server-common-lib/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	commandPort             = "--port=8511"
	commandExecFile         = "****"
	commandConfigFilePrefix = "--config-file="
	commandTokenFilePrefix  = "--token-path="
	commandDelToken         = "--del-token=false"
	commandHandlePath       = "--handle-path=gitcode-hook"
	configYaml              = "config.yaml"
)

func TestGatherOptions(t *testing.T) {

	args := []string{
		commandExecFile,
		commandPort,
		commandConfigFilePrefix + findTestdata(t, configYaml),
	}
	opt := new(robotOptions)
	_, _ = opt.gatherOptions(flag.NewFlagSet(args[0], flag.ExitOnError), args[1:]...)
	assert.Equal(t, true, opt.interrupt)
	assert.Equal(t, "webhook", opt.service.HandlePath)
	assert.Equal(t, 8511, opt.service.Port)

	args = []string{
		commandExecFile,
		commandConfigFilePrefix + findTestdata(t, "config11.yaml"),
		commandHandlePath,
	}
	opt = new(robotOptions)
	_, _ = opt.gatherOptions(flag.NewFlagSet(args[0], flag.ExitOnError), args[1:]...)
	assert.Equal(t, true, opt.interrupt)

	args = []string{
		commandExecFile,
		commandConfigFilePrefix + findTestdata(t, configYaml),
		commandHandlePath,
		commandTokenFilePrefix + "/token1",
		commandDelToken,
	}
	_, _ = opt.gatherOptions(flag.NewFlagSet(args[0], flag.ExitOnError), args[1:]...)
	assert.Equal(t, true, opt.interrupt)

	args[3] = commandConfigFilePrefix + findTestdata(t, "config1.yaml")
	_, _ = opt.gatherOptions(flag.NewFlagSet(args[0], flag.ExitOnError), args[1:]...)
	assert.Equal(t, true, opt.interrupt)

	args[3] = commandTokenFilePrefix + findTestdata(t, "token")
	opt = new(robotOptions)
	got, token := opt.gatherOptions(flag.NewFlagSet(args[0], flag.ExitOnError), args[1:]...)
	assert.Equal(t, false, opt.interrupt)
	assert.Equal(t, "gitcode-hook", opt.service.HandlePath)
	want := &configuration{}
	_ = utils.LoadFromYaml(findTestdata(t, configYaml), want)
	assert.Equal(t, *want, *got)
	assert.Equal(t, "1231****55324", string(token))
}
