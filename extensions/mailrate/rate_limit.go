/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package mailrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const OptionKey = "fork.mail_rate_limit"

const keyPrefix = "fork:mailrate:v1:"

type Kind string

const (
	KindRegistration Kind = "registration"
	KindOther        Kind = "other"
)

type Config struct {
	Enabled           bool `json:"enabled"`
	RegistrationLimit int  `json:"registration_limit"`
	HardLimit         int  `json:"hard_limit"`
}

var DefaultConfig = Config{
	Enabled:           true,
	RegistrationLimit: 200,
	HardLimit:         290,
}

var (
	ErrInvalidConfig            = errors.New("invalid daily email limit configuration")
	ErrRedisUnavailable         = errors.New("daily email quota store is unavailable")
	ErrRegistrationLimitReached = errors.New("daily registration email limit reached")
	ErrHardLimitReached         = errors.New("daily email limit reached")
)

type LimitError struct {
	Kind  Kind
	Limit int
	Count int
}

func (e *LimitError) Error() string {
	if e.Kind == KindRegistration {
		return fmt.Sprintf("daily registration email limit reached (%d recipients)", e.Limit)
	}
	return fmt.Sprintf("daily email limit reached (%d recipients)", e.Limit)
}

func (e *LimitError) Unwrap() error {
	if e.Kind == KindRegistration {
		return ErrRegistrationLimitReached
	}
	return ErrHardLimitReached
}

func DefaultConfigJSON() string {
	encoded, _ := json.Marshal(DefaultConfig)
	return string(encoded)
}

// ParseConfig accepts an empty value as disabled so direct callers and tests
// that bypass model.InitOptionMap keep the upstream behavior. The persisted
// default is supplied by model.InitOptionMap and enables the fork feature.
func ParseConfig(raw string) (Config, error) {
	if strings.TrimSpace(raw) == "" {
		return Config{}, nil
	}
	var config Config
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if c.RegistrationLimit <= 0 || c.RegistrationLimit >= c.HardLimit || c.HardLimit > 300 {
		return ErrInvalidConfig
	}
	return nil
}

func ValidateConfig(raw string) error {
	_, err := ParseConfig(raw)
	return err
}

var reserveScript = redis.NewScript(`
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local requested = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local next = current + requested
if next > limit then
  return {0, current}
end
redis.call("INCRBY", KEYS[1], requested)
redis.call("EXPIREAT", KEYS[1], ARGV[3])
return {1, next}
`)

func Reserve(ctx context.Context, client redis.Cmdable, rawConfig string, kind Kind, recipients int, now time.Time) error {
	config, err := ParseConfig(rawConfig)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return nil
	}
	if recipients <= 0 {
		return errors.New("invalid email recipient count")
	}
	if client == nil {
		return ErrRedisUnavailable
	}
	limit := config.HardLimit
	if kind == KindRegistration {
		limit = min(config.RegistrationLimit, config.HardLimit)
	}
	dayStart := now.UTC().Truncate(24 * time.Hour)
	key := keyPrefix + dayStart.Format("2006-01-02")
	expiresAt := dayStart.Add(24 * time.Hour).Unix()
	result, err := reserveScript.Run(ctx, client, []string{key}, recipients, limit, expiresAt).Slice()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}
	if len(result) != 2 {
		return ErrRedisUnavailable
	}
	allowed, ok := result[0].(int64)
	if !ok {
		return ErrRedisUnavailable
	}
	if allowed == 1 {
		return nil
	}
	count, _ := result[1].(int64)
	return &LimitError{Kind: kind, Limit: limit, Count: int(count)}
}
