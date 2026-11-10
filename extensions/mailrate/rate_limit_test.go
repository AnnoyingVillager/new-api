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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func testRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

func TestParseConfigUsesPersistedDefaultsAndRejectsUnsafeThresholds(t *testing.T) {
	config, err := ParseConfig(DefaultConfigJSON())
	require.NoError(t, err)
	require.Equal(t, DefaultConfig, config)

	for _, raw := range []string{
		`{"enabled":true,"registration_limit":0,"hard_limit":290}`,
		`{"enabled":true,"registration_limit":200,"hard_limit":200}`,
		`{"enabled":true,"registration_limit":299,"hard_limit":301}`,
		`{"enabled":true,"registration_limit":200}`,
		`not-json`,
	} {
		require.ErrorIs(t, ValidateConfig(raw), ErrInvalidConfig)
	}
}

func TestReserveAllowsExactThresholdsAndRejectsCrossingRequestsWithoutIncrement(t *testing.T) {
	_, client := testRedis(t)
	ctx := context.Background()
	now := time.Now().UTC()
	raw := DefaultConfigJSON()

	require.NoError(t, Reserve(ctx, client, raw, KindRegistration, 199, now))
	require.NoError(t, Reserve(ctx, client, raw, KindRegistration, 1, now))
	err := Reserve(ctx, client, raw, KindRegistration, 1, now)
	require.ErrorIs(t, err, ErrRegistrationLimitReached)

	require.NoError(t, Reserve(ctx, client, raw, KindOther, 89, now))
	require.NoError(t, Reserve(ctx, client, raw, KindOther, 1, now))
	err = Reserve(ctx, client, raw, KindOther, 1, now)
	require.ErrorIs(t, err, ErrHardLimitReached)

	key := keyPrefix + now.Format("2006-01-02")
	value, err := client.Get(ctx, key).Int()
	require.NoError(t, err)
	require.Equal(t, 290, value)
}

func TestReserveStartsANewWindowAtUTCMidnight(t *testing.T) {
	server, client := testRedis(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(24 * time.Hour)
	raw := `{"enabled":true,"registration_limit":1,"hard_limit":2}`

	require.NoError(t, Reserve(ctx, client, raw, KindOther, 2, now))
	require.ErrorIs(t, Reserve(ctx, client, raw, KindOther, 1, now), ErrHardLimitReached)

	server.FastForward(24 * time.Hour)
	require.NoError(t, Reserve(ctx, client, raw, KindOther, 1, now.Add(24*time.Hour)))
}

func TestReserveIsAtomicAcrossConcurrentSenders(t *testing.T) {
	_, client := testRedis(t)
	ctx := context.Background()
	now := time.Now().UTC()
	raw := `{"enabled":true,"registration_limit":100,"hard_limit":200}`

	var wg sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Reserve(ctx, client, raw, KindRegistration, 10, now)
		}()
	}
	wg.Wait()
	close(results)

	allowed := 0
	for err := range results {
		if err == nil {
			allowed++
			continue
		}
		require.ErrorIs(t, err, ErrRegistrationLimitReached)
	}
	require.Equal(t, 10, allowed)
	value, err := client.Get(ctx, keyPrefix+now.Format("2006-01-02")).Int()
	require.NoError(t, err)
	require.Equal(t, 100, value)
}

func TestReserveFailsClosedWhenEnabledWithoutRedis(t *testing.T) {
	err := Reserve(context.Background(), nil, DefaultConfigJSON(), KindOther, 1, time.Now())
	require.ErrorIs(t, err, ErrRedisUnavailable)
	require.False(t, errors.Is(err, ErrHardLimitReached))
}

func TestReserveIsDisabledWhenOptionIsEmpty(t *testing.T) {
	require.NoError(t, Reserve(context.Background(), nil, "", KindRegistration, 1, time.Now()))
}
