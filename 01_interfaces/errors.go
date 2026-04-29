package interfaces

import "errors"

//Sentinel errors are named errors.
//👉 Use errors.New once. All callers compare via errors.Is().
//
//This way we don’t compare strings - it’s more reliable and faster.
var (
	ErrOrderNotFound          = errors.New("order not found")
	ErrOrderAlreadyCancelled  = errors.New("order already cancelled")
	ErrOrderCannotBeConfirmed = errors.New("order cannot be confirmed: wrong status")
)
