package state

import (
	"fmt"
	"github.com/stader-labs/stader-node/shared/utils/eth2"
	penalty_tracker "github.com/stader-labs/stader-node/stader-lib/penalty-tracker"
	pool_utils "github.com/stader-labs/stader-node/stader-lib/pool-utils"
	socializing_pool "github.com/stader-labs/stader-node/stader-lib/socializing-pool"
	stader_config "github.com/stader-labs/stader-node/stader-lib/stader-config"
	stake_pool_manager "github.com/stader-labs/stader-node/stader-lib/stake-pool-manager"
	"math/big"
	"time"

	"github.com/stader-labs/stader-node/shared/services/beacon"
	"github.com/stader-labs/stader-node/shared/services/config"
	"github.com/stader-labs/stader-node/shared/utils/log"
	"github.com/stader-labs/stader-node/stader-lib/node"
	sd_collateral "github.com/stader-labs/stader-node/stader-lib/sd-collateral"

	"github.com/stader-labs/stader-node/stader-lib/contracts"
	"github.com/stader-labs/stader-node/stader-lib/stader"
	"github.com/stader-labs/stader-node/stader-lib/tokens"
	"github.com/stader-labs/stader-node/stader-lib/types"

	"github.com/ethereum/go-ethereum/common"
)

type NetworkDetails struct {
	// Network details

	// done
	SdPrice *big.Int
	// done
	TotalValidators *big.Int
	// done
	TotalOperators *big.Int
	// done
	TotalStakedSd *big.Int
	// done
	TotalStakedEthByNos *big.Int
	// done
	TotalEthxSupply *big.Int
	// done
	TotalStakedEthByUsers *big.Int

	// Validator specific info

	// done
	ActiveValidators *big.Int
	// done
	QueuedValidators *big.Int
	// done
	SlashedValidators *big.Int
	// done
	ExitingValidators *big.Int
	// done
	WithdrawnValidators *big.Int
	// done
	ValidatorStatusMap map[types.ValidatorPubkey]beacon.ValidatorStatus
	ValidatorInfoMap   map[types.ValidatorPubkey]types.ValidatorContractInfo

	// done
	CumulativePenalty *big.Int
	// done
	UnclaimedClRewards *big.Int
	// done
	UnclaimedNonSocializingPoolElRewards *big.Int

	ClaimedSocializingPoolElRewards   *big.Int
	ClaimedSocializingPoolSdRewards   *big.Int
	UnclaimedSocializingPoolElRewards *big.Int
	UnclaimedSocializingPoolSDRewards *big.Int
	// done
	NextSocializingPoolRewardCycle types.RewardCycleDetails

	// TODO - apr computation are yet to be decided
	EthApr *big.Int
	SdApr  *big.Int
}

type NetworkStateCache struct {
	// Block / slot for this state
	ElBlockNumber    uint64
	BeaconSlotNumber uint64
	BeaconConfig     beacon.Eth2Config

	StaderNetworkDetails NetworkDetails

	// Validator details
	ValidatorDetails map[types.ValidatorPubkey]beacon.ValidatorStatus

	// Internal fields
	log *log.ColorLogger
}

func CreateNetworkStateCache(
	cfg *config.StaderNodeConfig,
	ec stader.ExecutionClient,
	bc beacon.Client,
	log *log.ColorLogger,
	slotNumber uint64,
	beaconConfig beacon.Eth2Config,
	nodeAddress common.Address,
) (*NetworkStateCache, error) {
	prnAddress := cfg.GetPermissionlessNodeRegistryAddress()
	ptAddress := cfg.GetPenaltyTrackerAddress()
	sdcAddress := cfg.GetSdCollateralContractAddress()
	ethxAddress := cfg.GetEthxTokenAddress()
	stakePoolManagerAddress := cfg.GetStakePoolManagerAddress()
	poolUtilsAddress := cfg.GetPoolUtilsAddress()
	staderConfigAddress := cfg.GetStaderConfigAddress()
	socializingPoolAddress := cfg.GetSocializingPoolAddress()
	vaultFactoryAddress := cfg.GetVaultFactoryAddress()

	prn, err := stader.NewPermissionlessNodeRegistry(ec, prnAddress)
	if err != nil {
		return nil, err
	}
	sdc, err := stader.NewSdCollateralContract(ec, sdcAddress)
	if err != nil {
		return nil, err
	}
	ethx, err := stader.NewErc20TokenContract(ec, ethxAddress)
	if err != nil {
		return nil, err
	}
	pt, err := stader.NewPenaltyTracker(ec, ptAddress)
	if err != nil {
		return nil, err
	}
	spm, err := stader.NewStakePoolManager(ec, stakePoolManagerAddress)
	if err != nil {
		return nil, err
	}
	putils, err := stader.NewPoolUtils(ec, poolUtilsAddress)
	if err != nil {
		return nil, err
	}
	sdcfg, err := stader.NewStaderConfig(ec, staderConfigAddress)
	if err != nil {
		return nil, err
	}
	sp, err := stader.NewSocializingPool(ec, socializingPoolAddress)
	if err != nil {
		return nil, err
	}
	vf, err := stader.NewVaultFactory(ec, vaultFactoryAddress)
	if err != nil {
		return nil, err
	}

	// Get the execution block for the given slot
	beaconBlock, exists, err := bc.GetBeaconBlock(fmt.Sprintf("%d", slotNumber))
	if err != nil {
		return nil, fmt.Errorf("error getting Beacon block for slot %d: %w", slotNumber, err)
	}
	if !exists {
		return nil, fmt.Errorf("slot %d did not have a Beacon block", slotNumber)
	}

	// Get the corresponding block on the EL
	elBlockNumber := beaconBlock.ExecutionBlockNumber

	// Create the state wrapper
	state := &NetworkStateCache{
		BeaconSlotNumber: slotNumber,
		ElBlockNumber:    elBlockNumber,
		BeaconConfig:     beaconConfig,
		log:              log,
	}

	state.logLine("Getting network state for EL block %d, Beacon slot %d", elBlockNumber, slotNumber)

	start := time.Now()

	// fetch all validator pub keys
	operatorId, err := node.GetOperatorId(prn, nodeAddress, nil)
	if err != nil {
		return nil, err
	}
	operatorElRewardAddress, err := node.GetNodeElRewardAddress(vf, 1, operatorId, nil)
	if err != nil {
		return nil, err
	}
	elRewardAddressBalance, err := tokens.GetEthBalance(prn.Client, operatorElRewardAddress, nil)
	if err != nil {
		return nil, err
	}
	operatorElRewards, err := pool_utils.CalculateRewardShare(putils, 1, elRewardAddressBalance, nil)
	if err != nil {
		return nil, err
	}

	nextRewardCycleDetails, err := socializing_pool.GetRewardDetails(sp, nil)
	if err != nil {
		return nil, err
	}

	totalValidatorKeys, err := node.GetTotalValidatorKeys(prn, operatorId, nil)
	if err != nil {
		return nil, err
	}

	pubkeys := make([]types.ValidatorPubkey, 0, totalValidatorKeys.Int64())
	validatorInfoMap := map[types.ValidatorPubkey]types.ValidatorContractInfo{}
	for i := 0; i < int(totalValidatorKeys.Int64()); i++ {
		validatorId, err := node.GetValidatorIdByOperatorId(prn, operatorId, big.NewInt(int64(i)), nil)
		if err != nil {
			return nil, err
		}
		validatorInfo, err := node.GetValidatorInfo(prn, validatorId, nil)
		if err != nil {
			return nil, err
		}

		pubKey := types.BytesToValidatorPubkey(validatorInfo.Pubkey)
		validatorInfoMap[pubKey] = validatorInfo

		pubkeys = append(pubkeys, pubKey)
	}

	// Get the validator stats from Beacon
	statusMap, err := bc.GetValidatorStatuses(pubkeys, &beacon.ValidatorStatusOptions{
		Slot: &slotNumber,
	})
	if err != nil {
		return nil, err
	}
	state.ValidatorDetails = statusMap
	activeValidators := big.NewInt(0)
	slashedValidators := big.NewInt(0)
	queuedValidators := big.NewInt(0)
	exitingValidators := big.NewInt(0)
	withdrawnValidators := big.NewInt(0)
	totalClRewards := big.NewInt(0)
	cumulativePenalty := big.NewInt(0)
	for pubKey, status := range statusMap {
		log.Printlnf("pubkey: %s, status: %s", pubKey, status)
		if eth2.IsValidatorQueued(status) {
			queuedValidators.Add(queuedValidators, big.NewInt(1))
		}
		if eth2.IsValidatorSlashed(status) {
			slashedValidators.Add(slashedValidators, big.NewInt(1))
		}
		if eth2.IsValidatorExiting(status) {
			exitingValidators.Add(exitingValidators, big.NewInt(1))
		}
		if eth2.IsValidatorWithdrawn(status) {
			withdrawnValidators.Add(withdrawnValidators, big.NewInt(1))
		}
		if eth2.IsValidatorActive(status) {
			activeValidators.Add(activeValidators, big.NewInt(1))
		}

		validatorWithdrawVault := validatorInfoMap[pubKey].WithdrawVaultAddress
		withdrawVaultBalance, err := tokens.GetEthBalance(prn.Client, validatorWithdrawVault, nil)
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
		if withdrawVaultRewardShares.OperatorShare.Cmp(rewardsThreshold) > 0 {
			continue
		} else {
			totalClRewards.Add(totalClRewards, withdrawVaultRewardShares.OperatorShare)
		}

		totalValidatorPenalty, err := penalty_tracker.GetCumulativeValidatorPenalty(pt, pubKey, nil)
		if err != nil {
			return nil, err
		}
		cumulativePenalty.Add(cumulativePenalty, totalValidatorPenalty)
	}

	state.logLine("Retrieved validator details (total time: %s)", time.Since(start))

	state.logLine("Retrieved Socializing Pool Reward Details")

	start = time.Now()

	// TODO - compute these accuratley
	unclaimedSocializingPoolElRewards := big.NewInt(10)
	unclaimedSocializingPoolSdRewards := big.NewInt(20)
	claimedSocializingPoolElRewards := big.NewInt(30)
	claimedSocializingPoolSdRewards := big.NewInt(40)

	rewardDetails, err := socializing_pool.GetRewardDetails(sp, nil)
	if err != nil {
		return nil, err
	}
	for i := int64(1); i < rewardDetails.CurrentIndex.Int64(); i++ {

	}

	state.logLine("Retrieved Socializing Pool Reward Details (total time: %s)", time.Since(start))

	state.logLine("Getting Stader Network Details")

	start = time.Now()

	networkDetails := NetworkDetails{}

	sdPrice, err := sd_collateral.ConvertEthToSd(sdc, big.NewInt(1), nil)
	if err != nil {
		return nil, err
	}
	totalOperators, err := node.GetNextOperatorId(prn, nil)
	if err != nil {
		return nil, err
	}
	totalValidators, err := node.GetNextValidatorId(prn, nil)
	if err != nil {
		return nil, err
	}
	totalSdCollateral, err := sd_collateral.GetTotalSdCollateral(sdc, nil)
	if err != nil {
		return nil, err
	}
	ethxSupply, err := tokens.TotalSupply(ethx, nil)
	if err != nil {
		return nil, err
	}
	totalStakedAssets, err := stake_pool_manager.GetTotalAssets(spm, nil)
	if err != nil {
		return nil, err
	}

	networkDetails.SdPrice = sdPrice
	networkDetails.TotalOperators = totalOperators.Sub(totalOperators, big.NewInt(1))
	networkDetails.TotalValidators = totalValidators.Sub(totalValidators, big.NewInt(1))
	networkDetails.TotalStakedSd = totalSdCollateral
	networkDetails.TotalEthxSupply = ethxSupply
	networkDetails.TotalStakedEthByUsers = totalStakedAssets
	networkDetails.TotalStakedEthByNos = big.NewInt(0).Mul(totalValidators, big.NewInt(4))

	networkDetails.ValidatorStatusMap = statusMap
	networkDetails.ValidatorInfoMap = validatorInfoMap
	networkDetails.ActiveValidators = activeValidators
	networkDetails.QueuedValidators = queuedValidators
	networkDetails.ExitingValidators = exitingValidators
	networkDetails.SlashedValidators = slashedValidators
	networkDetails.WithdrawnValidators = withdrawnValidators
	networkDetails.CumulativePenalty = cumulativePenalty
	networkDetails.UnclaimedClRewards = totalClRewards
	networkDetails.NextSocializingPoolRewardCycle = nextRewardCycleDetails
	networkDetails.UnclaimedNonSocializingPoolElRewards = operatorElRewards.OperatorShare
	networkDetails.UnclaimedSocializingPoolElRewards = unclaimedSocializingPoolElRewards
	networkDetails.UnclaimedSocializingPoolSDRewards = unclaimedSocializingPoolSdRewards
	networkDetails.ClaimedSocializingPoolElRewards = claimedSocializingPoolElRewards
	networkDetails.ClaimedSocializingPoolSdRewards = claimedSocializingPoolSdRewards

	state.StaderNetworkDetails = networkDetails

	state.logLine("Retrieved Stader Network Details (total time: %s)", time.Since(start))

	return state, nil
}

// Logs a line if the logger is specified
func (s *NetworkStateCache) logLine(format string, v ...interface{}) {
	if s.log != nil {
		s.log.Printlnf(format, v...)
	}
}

func spInfo(sp *stader.SocializingPoolContractManager) (struct {
	ethRewardRemaining *big.Int
	sdRewardRemaining  *big.Int
	rewardChan         <-chan *contracts.SocializingPoolOperatorRewardsClaimed
}, error) {

	outstruct := new(struct {
		ethRewardRemaining *big.Int
		sdRewardRemaining  *big.Int
		rewardChan         <-chan *contracts.SocializingPoolOperatorRewardsClaimed
	})
	ethRewardRemaining, err := sp.SocializingPool.TotalOperatorETHRewardsRemaining(nil)
	if err != nil {
		return *outstruct, err
	}

	sdRewardRemaining, err := sp.SocializingPool.TotalOperatorSDRewardsRemaining(nil)
	if err != nil {
		return *outstruct, err
	}
	sink := make(chan *contracts.SocializingPoolOperatorRewardsClaimed)
	// var recipient []common.Address
	// _, err = sp.SocializingPool.WatchOperatorRewardsClaimed(nil, sink, recipient)
	// if err != nil {
	// 	return *outstruct, err
	// }

	outstruct.ethRewardRemaining = ethRewardRemaining
	outstruct.sdRewardRemaining = sdRewardRemaining
	outstruct.rewardChan = sink

	return *outstruct, nil
}
