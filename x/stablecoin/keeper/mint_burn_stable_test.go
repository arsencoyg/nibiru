package keeper_test

import (
	"testing"
	"time"

	"github.com/NibiruChain/nibiru/x/common"
	pricefeedTypes "github.com/NibiruChain/nibiru/x/pricefeed/types"
	"github.com/NibiruChain/nibiru/x/stablecoin/types"
	"github.com/NibiruChain/nibiru/x/testutil"

	"github.com/NibiruChain/nibiru/x/testutil/sample"

	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------------------------------
// MintStable
// ------------------------------------------------------------------

func TestMsgMint_ValidateBasic(t *testing.T) {
	testCases := []struct {
		name string
		msg  types.MsgMintStable
		err  error
	}{
		{
			name: "invalid address",
			msg: types.MsgMintStable{
				Creator: "invalid_address",
			},
			err: sdkerrors.ErrInvalidAddress,
		}, {
			name: "valid address",
			msg: types.MsgMintStable{
				Creator: sample.AccAddress().String(),
			},
		},
	}
	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			err := tc.msg.ValidateBasic()
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMsgMintStableResponse_HappyPath(t *testing.T) {
	accFundsGovAmount := sdk.NewCoin(common.GovDenom, sdk.NewInt(10_000))
	accFundsCollAmount := sdk.NewCoin(common.CollDenom, sdk.NewInt(900_000))
	neededGovFees := sdk.NewCoin(common.GovDenom, sdk.NewInt(20))      // 0.002 fee
	neededCollFees := sdk.NewCoin(common.CollDenom, sdk.NewInt(1_800)) // 0.002 fee

	accFundsAmt := sdk.NewCoins(
		accFundsGovAmount.Add(neededGovFees),
		accFundsCollAmount.Add(neededCollFees),
	)

	tests := []struct {
		name        string
		accFunds    sdk.Coins
		msgMint     types.MsgMintStable
		msgResponse types.MsgMintStableResponse
		govPrice    sdk.Dec
		collPrice   sdk.Dec
		supplyNIBI  sdk.Coin
		supplyNUSD  sdk.Coin
		err         error
	}{
		{
			name:     "Successful mint",
			accFunds: accFundsAmt,
			msgMint: types.MsgMintStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewCoin(common.StableDenom, sdk.NewInt(1_000_000)),
			},
			msgResponse: types.MsgMintStableResponse{
				Stable:    sdk.NewCoin(common.StableDenom, sdk.NewInt(1_000_000)),
				UsedCoins: sdk.NewCoins(accFundsCollAmount, accFundsGovAmount),
				FeesPayed: sdk.NewCoins(neededCollFees, neededGovFees),
			},
			govPrice:   sdk.MustNewDecFromStr("10"),
			collPrice:  sdk.MustNewDecFromStr("1"),
			supplyNIBI: sdk.NewCoin(common.GovDenom, sdk.NewInt(10)),
			// 10_000 - 20 (neededAmt - fees) - 10 (0.5 of fees from EFund are burned)
			supplyNUSD: sdk.NewCoin(common.StableDenom, sdk.NewInt(1_000_000)),
			err:        nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			nibiruApp, ctx := testutil.NewNibiruApp(true)
			acc, _ := sdk.AccAddressFromBech32(tc.msgMint.Creator)
			oracle := sample.AccAddress()

			// We get module account, to create it.
			nibiruApp.AccountKeeper.GetModuleAccount(ctx, types.StableEFModuleAccount)

			// Set up markets for the pricefeed keeper.
			priceKeeper := &nibiruApp.PriceKeeper
			pfParams := pricefeedTypes.Params{
				Markets: []pricefeedTypes.Market{
					{MarketID: common.GovStablePool, BaseAsset: common.GovDenom,
						QuoteAsset: common.StableDenom,
						Oracles:    []sdk.AccAddress{oracle}, Active: true},
					{MarketID: common.CollStablePool, BaseAsset: common.CollDenom,
						QuoteAsset: common.StableDenom,
						Oracles:    []sdk.AccAddress{oracle}, Active: true},
				}}
			priceKeeper.SetParams(ctx, pfParams)

			collRatio := sdk.MustNewDecFromStr("0.9")
			feeRatio := sdk.MustNewDecFromStr("0.002")
			feeRatioEF := sdk.MustNewDecFromStr("0.5")
			bonusRateRecoll := sdk.MustNewDecFromStr("0.002")
			nibiruApp.StablecoinKeeper.SetParams(
				ctx, types.NewParams(collRatio, feeRatio, feeRatioEF, bonusRateRecoll))

			// Post prices to each market with the oracle.
			priceExpiry := ctx.BlockTime().Add(time.Hour)
			_, err := priceKeeper.SetPrice(
				ctx, oracle, common.GovStablePool, tc.govPrice, priceExpiry,
			)
			require.NoError(t, err)
			_, err = priceKeeper.SetPrice(
				ctx, oracle, common.CollStablePool, tc.collPrice, priceExpiry,
			)
			require.NoError(t, err)

			// Update the 'CurrentPrice' posted by the oracles.
			for _, market := range pfParams.Markets {
				err = priceKeeper.SetCurrentPrices(ctx, market.MarketID)
				require.NoError(t, err, "Error posting price for market: %d", market)
			}

			// Fund account
			err = simapp.FundAccount(nibiruApp.BankKeeper, ctx, acc, tc.accFunds)
			require.NoError(t, err)

			// Mint NUSD -> Response contains Stable (sdk.Coin)
			goCtx := sdk.WrapSDKContext(ctx)
			mintStableResponse, err := nibiruApp.StablecoinKeeper.MintStable(
				goCtx, &tc.msgMint)

			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}

			require.NoError(t, err)
			testutil.RequireEqualWithMessage(
				t, *mintStableResponse, tc.msgResponse, "mintStableResponse")

			require.Equal(t, nibiruApp.StablecoinKeeper.GetSupplyNIBI(ctx), tc.supplyNIBI)
			require.Equal(t, nibiruApp.StablecoinKeeper.GetSupplyNUSD(ctx), tc.supplyNUSD)

			// Check balances in EF
			efModuleBalance := nibiruApp.BankKeeper.GetAllBalances(ctx, nibiruApp.AccountKeeper.GetModuleAddress(types.StableEFModuleAccount))
			collFeesInEf := neededCollFees.Amount.ToDec().Mul(sdk.MustNewDecFromStr("0.5")).TruncateInt()
			require.Equal(t, sdk.NewCoins(sdk.NewCoin(common.CollDenom, collFeesInEf)), efModuleBalance)

			// Check balances in Treasury
			treasuryModuleBalance := nibiruApp.BankKeeper.
				GetAllBalances(ctx, nibiruApp.AccountKeeper.GetModuleAddress(common.TreasuryPoolModuleAccount))
			collFeesInTreasury := neededCollFees.Amount.ToDec().Mul(sdk.MustNewDecFromStr("0.5")).TruncateInt()
			govFeesInTreasury := neededGovFees.Amount.ToDec().Mul(sdk.MustNewDecFromStr("0.5")).TruncateInt()
			require.Equal(
				t,
				sdk.NewCoins(
					sdk.NewCoin(common.CollDenom, collFeesInTreasury),
					sdk.NewCoin(common.GovDenom, govFeesInTreasury),
				),
				treasuryModuleBalance,
			)
		})
	}
}

func TestMsgMintStableResponse_NotEnoughFunds(t *testing.T) {
	testCases := []struct {
		name        string
		accFunds    sdk.Coins
		msgMint     types.MsgMintStable
		msgResponse types.MsgMintStableResponse
		govPrice    sdk.Dec
		collPrice   sdk.Dec
		err         error
	}{
		{
			name: "User has no GOV",
			accFunds: sdk.NewCoins(
				sdk.NewCoin(common.CollDenom, sdk.NewInt(9001)),
				sdk.NewCoin(common.GovDenom, sdk.NewInt(0)),
			),
			msgMint: types.MsgMintStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewCoin(common.StableDenom, sdk.NewInt(100)),
			},
			msgResponse: types.MsgMintStableResponse{
				Stable: sdk.NewCoin(common.StableDenom, sdk.NewInt(0)),
			},
			govPrice:  sdk.MustNewDecFromStr("10"),
			collPrice: sdk.MustNewDecFromStr("1"),
			err:       types.NotEnoughBalance.Wrap(common.GovDenom),
		}, {
			name: "User has no COLL",
			accFunds: sdk.NewCoins(
				sdk.NewCoin(common.CollDenom, sdk.NewInt(0)),
				sdk.NewCoin(common.GovDenom, sdk.NewInt(9001)),
			),
			msgMint: types.MsgMintStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewCoin(common.StableDenom, sdk.NewInt(100)),
			},
			msgResponse: types.MsgMintStableResponse{
				Stable: sdk.NewCoin(common.StableDenom, sdk.NewInt(0)),
			},
			govPrice:  sdk.MustNewDecFromStr("10"),
			collPrice: sdk.MustNewDecFromStr("1"),
			err:       types.NotEnoughBalance.Wrap(common.CollDenom),
		},
		{
			name: "Not enough GOV",
			accFunds: sdk.NewCoins(
				sdk.NewCoin(common.CollDenom, sdk.NewInt(9001)),
				sdk.NewCoin(common.GovDenom, sdk.NewInt(1)),
			),
			msgMint: types.MsgMintStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewCoin(common.StableDenom, sdk.NewInt(1000)),
			},
			msgResponse: types.MsgMintStableResponse{
				Stable: sdk.NewCoin(common.StableDenom, sdk.NewInt(0)),
			},
			govPrice:  sdk.MustNewDecFromStr("10"),
			collPrice: sdk.MustNewDecFromStr("1"),
			err: types.NotEnoughBalance.Wrap(
				sdk.NewCoin(common.GovDenom, sdk.NewInt(1)).String()),
		}, {
			name: "Not enough COLL",
			accFunds: sdk.NewCoins(
				sdk.NewCoin(common.CollDenom, sdk.NewInt(1)),
				sdk.NewCoin(common.GovDenom, sdk.NewInt(9001)),
			),
			msgMint: types.MsgMintStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewCoin(common.StableDenom, sdk.NewInt(100)),
			},
			msgResponse: types.MsgMintStableResponse{
				Stable: sdk.NewCoin(common.StableDenom, sdk.NewInt(0)),
			},
			govPrice:  sdk.MustNewDecFromStr("10"),
			collPrice: sdk.MustNewDecFromStr("1"),
			err: types.NotEnoughBalance.Wrap(
				sdk.NewCoin(common.CollDenom, sdk.NewInt(1)).String()),
		},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			nibiruApp, ctx := testutil.NewNibiruApp(true)
			acc, _ := sdk.AccAddressFromBech32(tc.msgMint.Creator)
			oracle := sample.AccAddress()

			// We get module account, to create it.
			nibiruApp.AccountKeeper.GetModuleAccount(ctx, types.StableEFModuleAccount)

			// Set up markets for the pricefeed keeper.
			priceKeeper := &nibiruApp.PriceKeeper
			pfParams := pricefeedTypes.Params{
				Markets: []pricefeedTypes.Market{
					{MarketID: common.GovStablePool,
						BaseAsset:  common.GovDenom,
						QuoteAsset: common.StableDenom,
						Oracles:    []sdk.AccAddress{oracle}, Active: true},
					{MarketID: common.CollStablePool,
						BaseAsset:  common.CollDenom,
						QuoteAsset: common.StableDenom,
						Oracles:    []sdk.AccAddress{oracle}, Active: true},
				}}
			priceKeeper.SetParams(ctx, pfParams)

			collRatio := sdk.MustNewDecFromStr("0.9")
			feeRatio := sdk.ZeroDec()
			feeRatioEF := sdk.MustNewDecFromStr("0.5")
			bonusRateRecoll := sdk.MustNewDecFromStr("0.002")
			nibiruApp.StablecoinKeeper.SetParams(
				ctx, types.NewParams(collRatio, feeRatio, feeRatioEF, bonusRateRecoll))

			// Post prices to each market with the oracle.
			priceExpiry := ctx.BlockTime().Add(time.Hour)
			_, err := priceKeeper.SetPrice(
				ctx, oracle, common.GovStablePool, tc.govPrice, priceExpiry,
			)
			require.NoError(t, err)
			_, err = priceKeeper.SetPrice(
				ctx, oracle, common.CollStablePool, tc.collPrice, priceExpiry,
			)
			require.NoError(t, err)

			// Update the 'CurrentPrice' posted by the oracles.
			for _, market := range pfParams.Markets {
				err = priceKeeper.SetCurrentPrices(ctx, market.MarketID)
				require.NoError(t, err, "Error posting price for market: %d", market)
			}

			// Fund account
			err = simapp.FundAccount(nibiruApp.BankKeeper, ctx, acc, tc.accFunds)
			require.NoError(t, err)

			// Mint NUSD -> Response contains Stable (sdk.Coin)
			goCtx := sdk.WrapSDKContext(ctx)
			mintStableResponse, err := nibiruApp.StablecoinKeeper.MintStable(
				goCtx, &tc.msgMint)

			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}

			require.NoError(t, err)
			testutil.RequireEqualWithMessage(
				t, *mintStableResponse, tc.msgResponse, "mintStableResponse")

			balances := nibiruApp.BankKeeper.GetAllBalances(ctx, nibiruApp.AccountKeeper.GetModuleAddress(types.StableEFModuleAccount))
			require.Equal(t, mintStableResponse.FeesPayed, balances)
		})
	}
}

// ------------------------------------------------------------------
// BurnStable / Redeem
// ------------------------------------------------------------------

func TestMsgBurn_ValidateBasic(t *testing.T) {
	testCases := []struct {
		name string
		msg  types.MsgBurnStable
		err  error
	}{
		{
			name: "invalid address",
			msg: types.MsgBurnStable{
				Creator: "invalid_address",
			},
			err: sdkerrors.ErrInvalidAddress,
		}, {
			name: "valid address",
			msg: types.MsgBurnStable{
				Creator: sample.AccAddress().String(),
			},
		},
	}
	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			err := tc.msg.ValidateBasic()
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMsgBurnResponse_NotEnoughFunds(t *testing.T) {
	tests := []struct {
		name         string
		accFunds     sdk.Coins
		moduleFunds  sdk.Coins
		msgBurn      types.MsgBurnStable
		msgResponse  *types.MsgBurnStableResponse
		govPrice     sdk.Dec
		collPrice    sdk.Dec
		expectedPass bool
		err          string
	}{
		{
			name:     "Not enough stable",
			accFunds: sdk.NewCoins(sdk.NewInt64Coin(common.StableDenom, 10)),
			msgBurn: types.MsgBurnStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewInt64Coin(common.StableDenom, 9001),
			},
			msgResponse: &types.MsgBurnStableResponse{
				Collateral: sdk.NewCoin(common.GovDenom, sdk.ZeroInt()),
				Gov:        sdk.NewCoin(common.CollDenom, sdk.ZeroInt()),
			},
			govPrice:     sdk.MustNewDecFromStr("10"),
			collPrice:    sdk.MustNewDecFromStr("1"),
			expectedPass: false,
			err:          "insufficient funds",
		},
		{
			name:      "Stable is zero",
			govPrice:  sdk.MustNewDecFromStr("10"),
			collPrice: sdk.MustNewDecFromStr("1"),
			accFunds: sdk.NewCoins(
				sdk.NewInt64Coin(common.StableDenom, 1000000000),
			),
			moduleFunds: sdk.NewCoins(
				sdk.NewInt64Coin(common.CollDenom, 100000000),
			),
			msgBurn: types.MsgBurnStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewCoin(common.StableDenom, sdk.ZeroInt()),
			},
			msgResponse: &types.MsgBurnStableResponse{
				Gov:        sdk.NewCoin(common.GovDenom, sdk.ZeroInt()),
				Collateral: sdk.NewCoin(common.CollDenom, sdk.ZeroInt()),
				FeesPayed:  sdk.NewCoins(),
			},
			expectedPass: true,
			err:          types.NoCoinFound.Wrap(common.StableDenom).Error(),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			nibiruApp, ctx := testutil.NewNibiruApp(true)
			acc, _ := sdk.AccAddressFromBech32(tc.msgBurn.Creator)
			oracle := sample.AccAddress()

			// Set stablecoin params
			collRatio := sdk.MustNewDecFromStr("0.9")
			feeRatio := sdk.MustNewDecFromStr("0.002")
			feeRatioEF := sdk.MustNewDecFromStr("0.5")
			bonusRateRecoll := sdk.MustNewDecFromStr("0.002")
			nibiruApp.StablecoinKeeper.SetParams(
				ctx, types.NewParams(collRatio, feeRatio, feeRatioEF, bonusRateRecoll))

			// Set up markets for the pricefeed keeper.
			priceKeeper := nibiruApp.PriceKeeper
			pfParams := pricefeedTypes.Params{
				Markets: []pricefeedTypes.Market{
					{MarketID: common.GovStablePool, BaseAsset: common.CollDenom, QuoteAsset: common.GovDenom,
						Oracles: []sdk.AccAddress{oracle}, Active: true},
					{MarketID: common.CollStablePool, BaseAsset: common.CollDenom, QuoteAsset: common.StableDenom,
						Oracles: []sdk.AccAddress{oracle}, Active: true},
				}}
			priceKeeper.SetParams(ctx, pfParams)

			nibiruApp.StablecoinKeeper.SetParams(ctx, types.DefaultParams())

			// Post prices to each market with the oracle.
			priceExpiry := ctx.BlockTime().Add(time.Hour)
			_, err := priceKeeper.SetPrice(
				ctx, oracle, common.GovStablePool, tc.govPrice, priceExpiry,
			)
			require.NoError(t, err)
			_, err = priceKeeper.SetPrice(
				ctx, oracle, common.CollStablePool, tc.collPrice, priceExpiry,
			)
			require.NoError(t, err)

			// Update the 'CurrentPrice' posted by the oracles.
			for _, market := range pfParams.Markets {
				err = priceKeeper.SetCurrentPrices(ctx, market.MarketID)
				require.NoError(t, err, "Error posting price for market: %d", market)
			}

			// Add collaterals to the module
			err = nibiruApp.BankKeeper.MintCoins(ctx, types.ModuleName, tc.moduleFunds)
			if err != nil {
				panic(err)
			}

			err = simapp.FundAccount(nibiruApp.BankKeeper, ctx, acc, tc.accFunds)
			require.NoError(t, err)

			// Burn NUSD -> Response contains GOV and COLL
			goCtx := sdk.WrapSDKContext(ctx)
			burnStableResponse, err := nibiruApp.StablecoinKeeper.BurnStable(
				goCtx, &tc.msgBurn)

			if !tc.expectedPass {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.err)

				return
			}
			require.NoError(t, err)
			testutil.RequireEqualWithMessage(
				t, burnStableResponse, tc.msgResponse, "burnStableResponse")
		})
	}
}

func TestMsgBurnResponse_HappyPath(t *testing.T) {
	tests := []struct {
		name          string
		accFunds      sdk.Coins
		moduleFunds   sdk.Coins
		msgBurn       types.MsgBurnStable
		msgResponse   types.MsgBurnStableResponse
		govPrice      sdk.Dec
		collPrice     sdk.Dec
		supplyNIBI    sdk.Coin
		supplyNUSD    sdk.Coin
		ecosystemFund sdk.Coins
		treasuryFund  sdk.Coins
		expectedPass  bool
		err           string
	}{
		{
			name:      "Happy path",
			govPrice:  sdk.MustNewDecFromStr("10"),
			collPrice: sdk.MustNewDecFromStr("1"),
			accFunds: sdk.NewCoins(
				sdk.NewInt64Coin(common.StableDenom, 1_000_000_000),
			),
			moduleFunds: sdk.NewCoins(
				sdk.NewInt64Coin(common.CollDenom, 100_000_000),
			),
			msgBurn: types.MsgBurnStable{
				Creator: sample.AccAddress().String(),
				Stable:  sdk.NewInt64Coin(common.StableDenom, 10_000_000),
			},
			msgResponse: types.MsgBurnStableResponse{
				Gov:        sdk.NewInt64Coin(common.GovDenom, 100_000-200),       // amount - fees 0,02%
				Collateral: sdk.NewInt64Coin(common.CollDenom, 9_000_000-18_000), // amount - fees 0,02%
				FeesPayed: sdk.NewCoins(
					sdk.NewInt64Coin(common.GovDenom, 200),
					sdk.NewInt64Coin(common.CollDenom, 18_000),
				),
			},
			supplyNIBI:    sdk.NewCoin(common.GovDenom, sdk.NewInt(100_000-100)), // nibiru minus 0.5 of fees burned (the part that goes to EF)
			supplyNUSD:    sdk.NewCoin(common.StableDenom, sdk.NewInt(1_000_000_000-10_000_000)),
			ecosystemFund: sdk.NewCoins(sdk.NewInt64Coin(common.CollDenom, 9000)),
			treasuryFund:  sdk.NewCoins(sdk.NewInt64Coin(common.CollDenom, 9000), sdk.NewInt64Coin(common.GovDenom, 100)),
			expectedPass:  true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			nibiruApp, ctx := testutil.NewNibiruApp(true)
			acc, _ := sdk.AccAddressFromBech32(tc.msgBurn.Creator)
			oracle := sample.AccAddress()

			// Set stablecoin params
			collRatio := sdk.MustNewDecFromStr("0.9")
			feeRatio := sdk.MustNewDecFromStr("0.002")
			feeRatioEF := sdk.MustNewDecFromStr("0.5")
			bonusRateRecoll := sdk.MustNewDecFromStr("0.002")
			nibiruApp.StablecoinKeeper.SetParams(
				ctx, types.NewParams(collRatio, feeRatio, feeRatioEF, bonusRateRecoll))

			// Set up markets for the pricefeed keeper.
			priceKeeper := nibiruApp.PriceKeeper
			pfParams := pricefeedTypes.Params{
				Markets: []pricefeedTypes.Market{
					{MarketID: common.GovStablePool, BaseAsset: common.CollDenom, QuoteAsset: common.GovDenom,
						Oracles: []sdk.AccAddress{oracle}, Active: true},
					{MarketID: common.CollStablePool, BaseAsset: common.CollDenom, QuoteAsset: common.StableDenom,
						Oracles: []sdk.AccAddress{oracle}, Active: true},
				}}
			priceKeeper.SetParams(ctx, pfParams)

			// Post prices to each market with the oracle.
			priceExpiry := ctx.BlockTime().Add(time.Hour)
			_, err := priceKeeper.SetPrice(
				ctx, oracle, common.GovStablePool, tc.govPrice, priceExpiry,
			)
			require.NoError(t, err)
			_, err = priceKeeper.SetPrice(
				ctx, oracle, common.CollStablePool, tc.collPrice, priceExpiry,
			)
			require.NoError(t, err)

			// Update the 'CurrentPrice' posted by the oracles.
			for _, market := range pfParams.Markets {
				err = priceKeeper.SetCurrentPrices(ctx, market.MarketID)
				require.NoError(t, err, "Error posting price for market: %d", market)
			}

			// Add collaterals to the module
			err = nibiruApp.BankKeeper.MintCoins(ctx, types.ModuleName, tc.moduleFunds)
			if err != nil {
				panic(err)
			}

			err = simapp.FundAccount(nibiruApp.BankKeeper, ctx, acc, tc.accFunds)
			require.NoError(t, err)

			// Burn NUSD -> Response contains GOV and COLL
			goCtx := sdk.WrapSDKContext(ctx)
			burnStableResponse, err := nibiruApp.StablecoinKeeper.BurnStable(
				goCtx, &tc.msgBurn)

			if !tc.expectedPass {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.err)

				return
			}
			require.NoError(t, err)
			testutil.RequireEqualWithMessage(
				t, burnStableResponse, &tc.msgResponse, "burnStableResponse")

			require.Equal(t, tc.supplyNIBI, nibiruApp.StablecoinKeeper.GetSupplyNIBI(ctx))
			require.Equal(t, tc.supplyNUSD, nibiruApp.StablecoinKeeper.GetSupplyNUSD(ctx))

			// Funds sypplies
			require.Equal(t, tc.ecosystemFund, nibiruApp.BankKeeper.GetAllBalances(ctx, nibiruApp.AccountKeeper.GetModuleAddress(types.StableEFModuleAccount)))
			require.Equal(t, tc.treasuryFund, nibiruApp.BankKeeper.GetAllBalances(ctx, nibiruApp.AccountKeeper.GetModuleAddress(common.TreasuryPoolModuleAccount)))
		})
	}
}