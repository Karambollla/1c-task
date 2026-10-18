package edition

import "edition/internal/application"

var _ application.Repository = (*Store)(nil)
