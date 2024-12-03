// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package badger

import (
	"fmt"
	"log/slog"
)

// logAdaptor is a simple slog adaptor so that badger will use the same logger as the rest of the code.
// Sadly this will not use slog fields as badger uses a printf style of logging.
type logAdaptor struct {
	Logger *slog.Logger
}

func (la logAdaptor) Errorf(format string, v ...interface{}) {
	la.Logger.Error(fmt.Sprintf(format, v...))
}

func (la logAdaptor) Warningf(format string, v ...interface{}) {
	la.Logger.Warn(fmt.Sprintf(format, v...))
}

func (la logAdaptor) Infof(format string, v ...interface{}) {
	la.Logger.Info(fmt.Sprintf(format, v...))
}

func (la logAdaptor) Debugf(format string, v ...interface{}) {
	la.Logger.Debug(fmt.Sprintf(format, v...))
}
