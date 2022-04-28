package pricefeed_test

import (
	"testing"
	"time"

	"github.com/NibiruChain/nibiru/x/pricefeed"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/NibiruChain/nibiru/app"
	"github.com/NibiruChain/nibiru/x/common"
	ptypes "github.com/NibiruChain/nibiru/x/pricefeed/types"
	"github.com/NibiruChain/nibiru/x/testutil"
	"github.com/NibiruChain/nibiru/x/testutil/sample"
	"github.com/stretchr/testify/require"
)

func TestTWAPriceUpdates(t *testing.T) {
	var app *app.NibiruApp
	var ctx sdk.Context

	runBlock := func(duration time.Duration) {
		ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 1).WithBlockTime(ctx.BlockTime().Add(duration))
		pricefeed.EndBlocker(ctx, app.PriceKeeper)
	}
	setPrice := func(price string) {
		_, err := app.PriceKeeper.SimSetPrice(
			ctx, common.StableDenom, common.CollDenom, sdk.MustNewDecFromStr(price), ctx.BlockTime().Add(time.Hour*5000*4))
		require.NoError(t, err)
	}

	app, ctx = testutil.NewNibiruApp(true)

	ctx = ctx.WithBlockTime(time.Date(2015, 10, 21, 0, 0, 0, 0, time.UTC))

	oracle := sample.AccAddress()
	markets := ptypes.NewParams([]ptypes.Pair{

		{
			Token0:  common.CollDenom,
			Token1:  common.StableDenom,
			Oracles: []sdk.AccAddress{oracle},
			Active:  true,
		},
	})

	app.PriceKeeper.SetParams(ctx, markets)

	// Sim set price set the price for one hour
	setPrice("0.9")

	err := app.StablecoinKeeper.SetCollRatio(ctx, sdk.MustNewDecFromStr("0.8"))
	require.NoError(t, err)

	// Pass 5000 hours, previous price is alive and we post a new one
	runBlock(time.Hour * 5000)
	setPrice("0.8")
	runBlock(time.Hour * 5000)

	/*
		New price should be.

		T0: 1463385600
		T1: 1481385600

		(0.9 * 1463385600 + (0.9 + 0.8) / 2 * 1481385600) / (1463385600 + 1481385600) = 0.8749844622444971
	*/
	price, err := app.PriceKeeper.GetCurrentTWAPPrice(ctx, common.StableDenom, common.CollDenom)
	require.NoError(t, err)
	priceFloat, err := price.Price.Float64()
	require.NoError(t, err)
	require.InDelta(t, 0.8748471867695528, priceFloat, 0.00000001)

	// 5000 hours passed, both previous price is alive and we post a new one
	setPrice("0.82")
	runBlock(time.Hour * 5000)

	/*
		New price should be.

		T0: 1463385600
		T1: 1481385600
		T1: 1499385600

		(0.9 * 1463385600 + (0.9 + 0.8) / 2 * 1481385600 + 0.82 * 1499385600) / (1463385600 + 1481385600 + 1499385600) = 0.8563426456960295
	*/
	price, err = app.PriceKeeper.GetCurrentTWAPPrice(ctx, common.StableDenom, common.CollDenom)
	require.NoError(t, err)
	priceFloat, err = price.Price.Float64()
	require.NoError(t, err)
	require.InDelta(t, 0.8563426456960295, priceFloat, 0.00000001)

	// 5000 hours passed, first price is now expired
	/*
		New price should be.

		T0: 1463385600
		T1: 1481385600
		T1: 1499385600
		T4: 1517385600

		(0.9 * 1463385600 + (0.9 + 0.8) / 2 * 1481385600 + 0.82 * 1499385600 + .82 * 1517385600) / (1463385600 + 1481385600 + 1499385600 + 1517385600) = 0.8470923873660615
	*/
	setPrice("0.83")
	runBlock(time.Hour * 5000)
	price, err = app.PriceKeeper.GetCurrentTWAPPrice(ctx, common.StableDenom, common.CollDenom)
	require.NoError(t, err)
	priceFloat, err = price.Price.Float64()
	require.NoError(t, err)
	require.InDelta(t, 0.8470923873660615, priceFloat, 0.00000001)
}