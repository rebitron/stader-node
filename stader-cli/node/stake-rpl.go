package node

import (
	"fmt"
	"github.com/rocket-pool/rocketpool-go/utils/eth"
	"github.com/urfave/cli"
	"math/big"

	"github.com/stader-labs/stader-node/shared/services/gas"
	"github.com/stader-labs/stader-node/shared/services/stader"
	cliutils "github.com/stader-labs/stader-node/shared/utils/cli"
	"github.com/stader-labs/stader-node/shared/utils/math"
)

func nodeDepositSd(c *cli.Context) error {

	// Get RP client
	staderClient, err := stader.NewClientFromCtx(c)
	if err != nil {
		return err
	}
	defer staderClient.Close()

	// Check and assign the EC status
	err = cliutils.CheckClientStatus(staderClient)
	if err != nil {
		return err
	}

	// Get node status
	//status, err := staderClient.NodeStatus()
	//if err != nil {
	//	return err
	//}

	// If a custom nonce is set, print the multi-transaction warning
	if c.GlobalUint64("nonce") != 0 {
		cliutils.PrintMultiTransactionNonceWarning()
	}

	//sdBalance := *(status.AccountBalances.Sd)

	// Get stake mount
	var amountWei *big.Int

	// Check allowance
	allowance, err := staderClient.GetNodeDepositSdAllowance()
	if err != nil {
		return err
	}

	if allowance.Allowance.Cmp(amountWei) < 0 {
		fmt.Println("Before depositing SD, you must first give the collateral contract approval to interact with your SD.")
		fmt.Println("This only needs to be done once for your node.")

		// If a custom nonce is set, print the multi-transaction warning
		if c.GlobalUint64("nonce") != 0 {
			cliutils.PrintMultiTransactionNonceWarning()
		}

		// Calculate max uint256 value
		maxApproval := big.NewInt(2)
		maxApproval = maxApproval.Exp(maxApproval, big.NewInt(256), nil)
		maxApproval = maxApproval.Sub(maxApproval, big.NewInt(1))

		// Get approval gas
		approvalGas, err := staderClient.NodeDepositSdApprovalGas(maxApproval)
		if err != nil {
			return err
		}
		// Assign max fees
		err = gas.AssignMaxFeeAndLimit(approvalGas.GasInfo, staderClient, c.Bool("yes"))
		if err != nil {
			return err
		}

		// Prompt for confirmation
		if !(c.Bool("yes") || cliutils.Confirm("Do you want to deposit your SD into the Collateral contract")) {
			fmt.Println("Cancelled.")
			return nil
		}

		// Approve RPL for staking
		response, err := staderClient.NodeDepositSdApprove(maxApproval)
		if err != nil {
			return err
		}
		hash := response.ApproveTxHash
		fmt.Printf("Approving RPL for staking...\n")
		cliutils.PrintTransactionHash(staderClient, hash)
		if _, err = staderClient.WaitForTransaction(hash); err != nil {
			return err
		}
		fmt.Println("Successfully approved staking access to RPL.")

		// If a custom nonce is set, increment it for the next transaction
		if c.GlobalUint64("nonce") != 0 {
			staderClient.IncrementCustomNonce()
		}
	}

	// Check RPL can be staked
	canStake, err := staderClient.CanNodeDepositSd(amountWei)
	if err != nil {
		return err
	}
	if !canStake.CanStake {
		fmt.Println("Cannot deposit SD:")
		if canStake.InsufficientBalance {
			fmt.Println("The node's SD balance is insufficient.")
		}
		return nil
	}

	// Assign max fees
	err = gas.AssignMaxFeeAndLimit(canStake.GasInfo, staderClient, c.Bool("yes"))
	if err != nil {
		return err
	}

	// Prompt for confirmation
	if !(c.Bool("yes") || cliutils.Confirm(fmt.Sprintf("Are you sure you want to Deposit %.6f SD? You will not be able to withdraw this SD until you exit your validators", math.RoundDown(eth.WeiToEth(amountWei), 6)))) {
		fmt.Println("Cancelled.")
		return nil
	}

	// Stake RPL
	stakeResponse, err := staderClient.NodeDepositSd(amountWei)
	if err != nil {
		return err
	}

	fmt.Printf("Deposting SD...\n")
	cliutils.PrintTransactionHash(staderClient, stakeResponse.StakeTxHash)
	if _, err = staderClient.WaitForTransaction(stakeResponse.StakeTxHash); err != nil {
		return err
	}

	// Log & return
	fmt.Printf("Successfully staked %.6f RPL.\n", math.RoundDown(eth.WeiToEth(amountWei), 6))
	return nil

}
