package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ----------------------------------------------------------
// Vpool Interface
// ----------------------------------------------------------

type VirtualPoolDirection uint8

const (
	VirtualPoolDirection_AddToAMM = iota
	VirtualPoolDirection_RemoveFromAMM
)

type IVirtualPool interface {
	Pair() string
	QuoteTokenDenom() string
	SwapInput(
		ctx sdk.Context, ammDir VirtualPoolDirection, inputAmount,
		minOutputAmount sdk.Int, canOverFluctuationLimit bool,
	) (sdk.Int, error)
	SwapOutput(
		ctx sdk.Context, dir VirtualPoolDirection, abs sdk.Int, limit sdk.Int,
	) (sdk.Int, error)
	GetOpenInterestNotionalCap(ctx sdk.Context) (sdk.Int, error)
	GetMaxHoldingBaseAsset(ctx sdk.Context) (sdk.Int, error)
	GetOutputTWAP(ctx sdk.Context, dir VirtualPoolDirection, abs sdk.Int,
	) (sdk.Int, error)
	GetOutputPrice(ctx sdk.Context, dir VirtualPoolDirection, abs sdk.Int,
	) (sdk.Int, error)
	GetUnderlyingPrice(ctx sdk.Context) (sdk.Dec, error)
	GetSpotPrice(ctx sdk.Context) (sdk.Int, error)
	// Inside the perp keeper for now, will be moved once vamm is finished
	//CalcFee(ctx sdk.Context, quoteAmt sdk.Int) (toll sdk.Int, spread sdk.Int, err error)
}