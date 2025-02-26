/*
This work is licensed and released under GNU GPL v3 or any other later versions.
The full text of the license is below/ found at <http://www.gnu.org/licenses/>

(c) 2023 Rocket Pool Pty Ltd. Modified under GNU GPL v3. [1.2.0]

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/
package validator

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/stader-labs/stader-node/shared/types/eth2"
	eth2types "github.com/wealdtech/go-eth2-types/v2"

	"github.com/stader-labs/stader-node/shared/services/beacon"
)

// Get deposit data & root for a given validator key and withdrawal credentials
func GetDepositData(validatorKey *eth2types.BLSPrivateKey, withdrawalCredentials common.Hash, eth2Config beacon.Eth2Config, amount uint64) (eth2.DepositData, common.Hash, error) {
	// Build deposit data
	dd := eth2.DepositDataNoSignature{
		PublicKey:             validatorKey.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials[:],
		Amount:                amount,
	}

	// Get signing root
	or, err := dd.HashTreeRoot()
	if err != nil {
		return eth2.DepositData{}, common.Hash{}, err
	}

	sr := eth2.SigningRoot{
		ObjectRoot: or[:],
		Domain:     eth2types.Domain(eth2types.DomainDeposit, eth2Config.GenesisForkVersion, eth2types.ZeroGenesisValidatorsRoot),
	}

	// Get signing root with domain
	srHash, err := sr.HashTreeRoot()
	if err != nil {
		return eth2.DepositData{}, common.Hash{}, err
	}

	// Build deposit data struct (with signature)
	var depositData = eth2.DepositData{
		PublicKey:             dd.PublicKey,
		WithdrawalCredentials: dd.WithdrawalCredentials,
		Amount:                dd.Amount,
		Signature:             validatorKey.Sign(srHash[:]).Marshal(),
	}

	// Get deposit data root
	depositDataRoot, err := depositData.HashTreeRoot()
	if err != nil {
		return eth2.DepositData{}, common.Hash{}, err
	}

	// Return
	return depositData, depositDataRoot, nil

}
