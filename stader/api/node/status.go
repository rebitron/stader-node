package node

import (
	pool_utils "github.com/stader-labs/stader-node/stader-lib/pool-utils"
	stader_config "github.com/stader-labs/stader-node/stader-lib/stader-config"
	"github.com/stader-labs/stader-node/stader-lib/types"
	"math/big"

	"github.com/stader-labs/stader-node/shared/services"
	"github.com/stader-labs/stader-node/shared/types/api"
	"github.com/stader-labs/stader-node/shared/utils/stdr"
	"github.com/stader-labs/stader-node/stader-lib/node"
	sd_collateral "github.com/stader-labs/stader-node/stader-lib/sd-collateral"
	"github.com/stader-labs/stader-node/stader-lib/tokens"
	"github.com/urfave/cli"
)

func getStatus(c *cli.Context) (*api.NodeStatusResponse, error) {

	// Get services
	if err := services.RequireNodeWallet(c); err != nil {
		return nil, err
	}
	w, err := services.GetWallet(c)
	if err != nil {
		return nil, err
	}
	pnr, err := services.GetPermissionlessNodeRegistry(c)
	if err != nil {
		return nil, err
	}
	sdt, err := services.GetSdTokenContract(c)
	if err != nil {
		return nil, err
	}
	sdc, err := services.GetSdCollateralContract(c)
	if err != nil {
		return nil, err
	}
	vf, err := services.GetVaultFactory(c)
	if err != nil {
		return nil, err
	}
	sdcfg, err := services.GetStaderConfigContract(c)
	if err != nil {
		return nil, err
	}
	putils, err := services.GetPoolUtilsContract(c)
	if err != nil {
		return nil, err
	}
	pp, err := services.GetPermissionlessPoolContract(c)
	if err != nil {
		return nil, err
	}
	bc, err := services.GetBeaconClient(c)
	if err != nil {
		return nil, err
	}

	// Response
	response := api.NodeStatusResponse{}

	nodeAccount, err := w.GetNodeAccount()
	if err != nil {
		return nil, err
	}

	response.AccountAddress = nodeAccount.Address

	accountEthBalance, err := tokens.GetEthBalance(pnr.Client, nodeAccount.Address, nil)
	if err != nil {
		return nil, err
	}
	accountSdBalance, err := tokens.BalanceOf(sdt, nodeAccount.Address, nil)
	if err != nil {
		return nil, err
	}

	response.AccountBalances.ETH = accountEthBalance
	response.AccountBalances.Sd = accountSdBalance

	socializingPoolAddress, err := node.GetSocializingPoolContract(pp, nil)
	if err != nil {
		return nil, err
	}
	response.SocializingPoolAddress = socializingPoolAddress

	operatorId, err := node.GetOperatorId(pnr, nodeAccount.Address, nil)
	if err != nil {
		return nil, err
	}
	operatorRegistry, err := node.GetOperatorInfo(pnr, operatorId, nil)
	if err != nil {
		return nil, err
	}

	if operatorRegistry.OperatorName != "" {
		response.Registered = true
		response.OperatorId = operatorId
		response.OperatorName = operatorRegistry.OperatorName
		response.OperatorActive = operatorRegistry.Active
		response.OperatorAddress = operatorRegistry.OperatorAddress
		response.OperatorRewardAddress = operatorRegistry.OperatorRewardAddress
		response.OptedInForSocializingPool = operatorRegistry.OptedForSocializingPool

		// non socializing pool fee recepient
		operatorElRewardAddress, err := node.GetNodeElRewardAddress(vf, 1, operatorId, nil)
		if err != nil {
			return nil, err
		}
		elRewardAddressBalance, err := tokens.GetEthBalance(pnr.Client, operatorElRewardAddress, nil)
		if err != nil {
			return nil, err
		}
		operatorElRewards, err := pool_utils.CalculateRewardShare(putils, 1, elRewardAddressBalance, nil)
		if err != nil {
			return nil, err
		}
		response.OperatorELRewardsAddress = operatorElRewardAddress
		response.OperatorELRewardsAddressBalance = operatorElRewards.OperatorShare

		operatorReward, err := tokens.GetEthBalance(pnr.Client, operatorRegistry.OperatorRewardAddress, nil)
		if err != nil {
			return nil, err
		}
		response.OperatorRewardInETH = operatorReward

		// get operator deposited sd collateral
		operatorSdCollateral, err := sd_collateral.GetOperatorSdBalance(sdc, nodeAccount.Address, nil)
		if err != nil {
			return nil, err
		}
		response.DepositedSdCollateral = operatorSdCollateral

		// total registerable validators
		totalSdWorthValidators, err := sd_collateral.GetMaxValidatorSpawnable(sdc, operatorSdCollateral, 1, nil)
		if err != nil {
			return nil, err
		}
		response.SdCollateralWorthValidators = totalSdWorthValidators

		// get sd collateral in unbonding phase
		withdrawReqSd, err := sd_collateral.GetOperatorWithdrawInfo(sdc, nodeAccount.Address, nil)
		if err != nil {
			return nil, err
		}
		withdrawDelay, err := sd_collateral.GetWithdrawDelay(sdc, nil)
		if err != nil {
			return nil, err
		}
		response.SdCollateralRequestedToWithdraw = withdrawReqSd.TotalSDWithdrawReqAmount
		response.SdCollateralWithdrawTime = withdrawReqSd.LastWithdrawReqTimestamp.Add(withdrawReqSd.LastWithdrawReqTimestamp, withdrawDelay.Add(withdrawDelay, big.NewInt(20)))

		totalValidatorKeys, err := node.GetTotalValidatorKeys(pnr, operatorId, nil)
		if err != nil {
			return nil, err
		}

		totalNonTerminalValidatorKeys, err := node.GetTotalNonTerminalValidatorKeys(pnr, nodeAccount.Address, totalValidatorKeys, nil)
		if err != nil {
			return nil, err
		}

		response.TotalNonTerminalValidators = big.NewInt(int64(totalNonTerminalValidatorKeys))

		validatorInfoArray := make([]stdr.ValidatorInfo, totalValidatorKeys.Int64())

		for i := int64(0); i < totalValidatorKeys.Int64(); i++ {
			validatorIndex, err := node.GetValidatorIdByOperatorId(pnr, operatorId, big.NewInt(i), nil)
			if err != nil {
				return nil, err
			}
			validatorContractInfo, err := node.GetValidatorInfo(pnr, validatorIndex, nil)
			if err != nil {
				return nil, err
			}
			withdrawVaultBalance, err := tokens.GetEthBalance(pnr.Client, validatorContractInfo.WithdrawVaultAddress, nil)
			if err != nil {
				return nil, err
			}
			withdrawVaultRewardShares, err := pool_utils.CalculateRewardShare(putils, 1, withdrawVaultBalance, nil)
			if err != nil {
				return nil, err
			}
			rewardsThreshold, err := stader_config.GetRewardsThreshold(sdcfg, nil)
			if err != nil {
				return nil, err
			}
			crossedRewardThreshold := false

			validatorWithdrawVaultWithdrawShares := big.NewInt(0)
			if withdrawVaultBalance.Cmp(rewardsThreshold) > 0 {
				withdrawVaultWithdrawShares, err := node.CalculateValidatorWithdrawVaultWithdrawShare(pnr.Client, validatorContractInfo.WithdrawVaultAddress, nil)
				if err != nil {
					return nil, err
				}
				validatorWithdrawVaultWithdrawShares = withdrawVaultWithdrawShares.OperatorShare
				crossedRewardThreshold = true
			}

			validatorBeaconStatus, err := bc.GetValidatorStatus(types.BytesToValidatorPubkey(validatorContractInfo.Pubkey), nil)
			if err != nil {
				return nil, err
			}

			validatorDisplayStatus, err := stdr.GetValidatorRunningStatus(validatorBeaconStatus, validatorContractInfo)
			if err != nil {
				return nil, err
			}

			validatorInfo := stdr.ValidatorInfo{
				Status:                           validatorContractInfo.Status,
				StatusToDisplay:                  validatorDisplayStatus,
				Pubkey:                           validatorContractInfo.Pubkey,
				PreDepositSignature:              validatorContractInfo.PreDepositSignature,
				DepositSignature:                 validatorContractInfo.DepositSignature,
				WithdrawVaultAddress:             validatorContractInfo.WithdrawVaultAddress,
				WithdrawVaultRewardBalance:       withdrawVaultRewardShares.OperatorShare,
				CrossedRewardsThreshold:          crossedRewardThreshold,
				WithdrawVaultWithdrawableBalance: validatorWithdrawVaultWithdrawShares,
				OperatorId:                       validatorContractInfo.OperatorId,
				DepositBlock:                     validatorContractInfo.DepositBlock,
				WithdrawnBlock:                   validatorContractInfo.WithdrawnBlock,
			}

			validatorInfoArray[i] = validatorInfo
		}

		response.ValidatorInfos = validatorInfoArray

	} else {
		response.DepositedSdCollateral = big.NewInt(0)
		response.SdCollateralWorthValidators = big.NewInt(0)
		response.ValidatorInfos = []stdr.ValidatorInfo{}
		response.Registered = false
	}

	// Return response
	return &response, nil
}
