package operator

import (
	"AAStarCommunity/EthPaymaster_BackService/common/model"
	"AAStarCommunity/EthPaymaster_BackService/common/types"
	"AAStarCommunity/EthPaymaster_BackService/common/utils"

	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/xerrors"
	"math/big"
	"strconv"
	"strings"
)

var (
	chainId = big.NewInt(11155111)
)

func TryPayUserOpExecute(request *model.TryPayUserOpRequest) (*model.TryPayUserOpResponse, error) {

	var strategy *model.Strategy
	// getStrategy
	strategy, generateErr := strategyGenerate(request)
	if generateErr != nil {
		return nil, generateErr
	}
	if strategy.EntryPointTag != types.EntrypointV06 {
		return nil, xerrors.Errorf("Not Support EntryPointTag: [%w]", strategy.EntryPointTag)
	}

	userOp, newUserOpError := model.NewUserOp(&request.UserOp)
	if newUserOpError != nil {
		return nil, newUserOpError
	}

	gasResponse := getMockGasResponse()

	var paymasterAndData string
	var paymasterSignature string
	if paymasterAndDataRes, paymasterSignatureRes, err := getPayMasterAndData(strategy, userOp, gasResponse); err != nil {
		return nil, err
	} else {
		paymasterAndData = paymasterAndDataRes
		paymasterSignature = paymasterSignatureRes
	}

	//validatePaymasterUserOp
	var result = &model.TryPayUserOpResponse{
		StrategyId:         strategy.Id,
		EntryPointAddress:  strategy.EntryPointAddress,
		PayMasterAddress:   strategy.PayMasterAddress,
		PayMasterSignature: paymasterSignature,
		PayMasterAndData:   paymasterAndData,
		GasInfo:            gasResponse,
	}
	return result, nil
}
func getMockGasResponse() *model.ComputeGasResponse {
	return &model.ComputeGasResponse{
		GasInfo:    &model.GasPrice{},
		TokenCost:  big.NewFloat(0),
		Network:    types.Sepolia,
		Token:      types.ETH,
		UsdCost:    0,
		BlobEnable: false,
		MaxFee:     *big.NewInt(1000000000),
	}

}

func getPayMasterSignature(strategy *model.Strategy, userOp *model.UserOperation) string {
	signatureBytes, _ := utils.SignUserOp("1d8a58126e87e53edc7b24d58d1328230641de8c4242c135492bf5560e0ff421", userOp)
	return hex.EncodeToString(signatureBytes)
}
func packUserOp(userOp *model.UserOperation) (string, []byte, error) {
	abiEncoder, err := abi.JSON(strings.NewReader(`[
    {
        "inputs": [
            {
                "components": [
                    {
                        "internalType": "address",
                        "name": "Sender",
                        "type": "address"
                    },
                    {
                        "internalType": "uint256",
                        "name": "Nonce",
                        "type": "uint256"
                    },
                    {
                        "internalType": "bytes",
                        "name": "InitCode",
                        "type": "bytes"
                    },
                    {
                        "internalType": "bytes",
                        "name": "CallData",
                        "type": "bytes"
                    },
                    {
                        "internalType": "uint256",
                        "name": "CallGasLimit",
                        "type": "uint256"
                    },
                    {
                        "internalType": "uint256",
                        "name": "VerificationGasLimit",
                        "type": "uint256"
                    },
                    {
                        "internalType": "uint256",
                        "name": "PreVerificationGas",
                        "type": "uint256"
                    },
                    {
                        "internalType": "uint256",
                        "name": "MaxFeePerGas",
                        "type": "uint256"
                    },
                    {
                        "internalType": "uint256",
                        "name": "MaxPriorityFeePerGas",
                        "type": "uint256"
                    },
                    {
                        "internalType": "bytes",
                        "name": "PaymasterAndData",
                        "type": "bytes"
                    },
                    {
                        "internalType": "bytes",
                        "name": "Signature",
                        "type": "bytes"
                    }
                ],
                "internalType": "struct UserOperation",
                "name": "userOp",
                "type": "tuple"
            }
        ],
        "name": "UserOp",
        "outputs": [],
        "stateMutability": "nonpayable",
        "type": "function"
    }
	]`))
	if err != nil {
		return "", nil, err
	}
	method := abiEncoder.Methods["UserOp"]
	//TODO disgusting logic

	paymasterDataTmp, err := hex.DecodeString("d93349Ee959d295B115Ee223aF10EF432A8E8523000000000000000000000000000000000000000000000000000000001710044496000000000000000000000000000000000000000000000000000000174158049605bea0bfb8539016420e76749fda407b74d3d35c539927a45000156335643827672fa359ee968d72db12d4b4768e8323cd47443505ab138a525c1f61c6abdac501")
	//fmt.Printf("paymasterDataTmpLen: %x\n", len(paymasterDataTmp))
	//fmt.Printf("paymasterDataKLen : %x\n", len(userOp.PaymasterAndData))
	userOp.PaymasterAndData = paymasterDataTmp
	encoded, err := method.Inputs.Pack(userOp)

	if err != nil {
		return "", nil, err
	}
	//https://github.com/jayden-sudo/SoulWalletCore/blob/dc76bdb9a156d4f99ef41109c59ab99106c193ac/contracts/utils/CalldataPack.sol#L51-L65
	hexString := hex.EncodeToString(encoded)

	//1. 从 63*10+ 1 ～64*10获取
	hexString = hexString[64:]
	//hexLen := len(hexString)
	subIndex := GetIndex(hexString)
	hexString = hexString[:subIndex]
	//fmt.Printf("subIndex: %d\n", subIndex)
	return hexString, encoded, nil
}
func GetIndex(hexString string) int64 {
	//1. 从 63*10+ 1 ～64*10获取

	indexPre := hexString[576:640]
	indePreInt, _ := strconv.ParseInt(indexPre, 16, 64)
	result := indePreInt * 2
	return result
}

func UserOpHash(userOp *model.UserOperation, strategy *model.Strategy, validStart *big.Int, validEnd *big.Int) ([]byte, string, error) {
	packUserOpStr, _, err := packUserOp(userOp)
	if err != nil {
		return nil, "", err
	}
	//
	bytesTy, err := abi.NewType("bytes", "", nil)
	if err != nil {
		fmt.Println(err)
	}
	uint256Ty, err := abi.NewType("uint256", "", nil)
	if err != nil {
		fmt.Println(err)
	}
	uint48Ty, err := abi.NewType("uint48", "", nil)

	addressTy, _ := abi.NewType("address", "", nil)
	arguments := abi.Arguments{
		{
			Type: bytesTy,
		},
		{
			Type: uint256Ty,
		},
		{
			Type: addressTy,
		},
		{
			Type: uint256Ty,
		},
		{
			Type: uint48Ty,
		},
		{
			Type: uint48Ty,
		},
	}
	if err != nil {
		return nil, "", err
	}
	packUserOpStrByteNew, _ := hex.DecodeString(packUserOpStr)

	bytesRes, err := arguments.Pack(packUserOpStrByteNew, chainId.Int64(), common.HexToAddress(strategy.PayMasterAddress), userOp.Nonce, validStart, validEnd)
	if err != nil {
		return nil, "", err
	}
	//bytesResStr := hex.EncodeToString(bytesRes)
	//fmt.Printf("bytesResStr: %s\n", bytesResStr)
	//fmt.Printf("bytesRes: %x\n", bytesRes)

	encodeHash := crypto.Keccak256(bytesRes)
	return encodeHash, hex.EncodeToString(bytesRes), nil

}

func getPayMasterAndData(strategy *model.Strategy, userOp *model.UserOperation, gasResponse *model.ComputeGasResponse) (string, string, error) {
	return generatePayMasterAndData(userOp, strategy)
}

func generatePayMasterAndData(userOp *model.UserOperation, strategy *model.Strategy) (string, string, error) {
	//v0.7 [0:20)paymaster address,[20:36)validation gas, [36:52)postop gas,[52:53)typeId,  [53:117)valid timestamp, [117:) signature
	//v0.6 [0:20)paymaster address,[20:22)payType, [22:86)start Time ,[86:150)typeId,  [53:117)valid timestamp, [117:) signature
	//validationGas := userOp.VerificationGasLimit.String()
	//postOPGas := userOp.CallGasLimit.String()
	validStart, validEnd := getValidTime()
	//fmt.Printf("validStart: %s, validEnd: %s\n", validStart, validEnd)
	//TODO  string(strategy.PayType),
	message := fmt.Sprintf("%s%s%s", strategy.PayMasterAddress, validEnd, validStart)
	signatureByte, _, err := SignPaymaster(userOp, strategy, validStart, validEnd)
	if err != nil {
		return "", "", err
	}
	signatureStr := hex.EncodeToString(signatureByte)
	message = message + signatureStr
	return message, signatureStr, nil
}

func SignPaymaster(userOp *model.UserOperation, strategy *model.Strategy, validStart string, validEnd string) ([]byte, []byte, error) {
	//string to int
	//TODO
	userOpHash, _, err := UserOpHash(userOp, strategy, big.NewInt(1820044496), big.NewInt(1710044496))
	hashToEthSignHash := utils.ToEthSignedMessageHash(userOpHash)
	fmt.Printf("userOpHashStr: %s\n", hex.EncodeToString(userOpHash))
	fmt.Printf("hashToEthSignHashStr: %s\n", hex.EncodeToString(hashToEthSignHash))
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := crypto.HexToECDSA("1d8a58126e87e53edc7b24d58d1328230641de8c4242c135492bf5560e0ff421")
	if err != nil {
		return nil, nil, err
	}

	signature, err := crypto.Sign(hashToEthSignHash, privateKey)

	signatureStr := hex.EncodeToString(signature)
	var signatureAfterProcess string

	if strings.HasSuffix(signatureStr, "00") {
		signatureAfterProcess = utils.ReplaceLastTwoChars(signatureStr, "1b")
	} else if strings.HasSuffix(signatureStr, "01") {
		signatureAfterProcess = utils.ReplaceLastTwoChars(signatureStr, "1c")
	} else {
		signatureAfterProcess = signatureStr
	}

	signatureAfterProcessByte, err := hex.DecodeString(signatureAfterProcess)
	if err != nil {
		return nil, nil, err
	}

	return signatureAfterProcessByte, userOpHash, err
}

// 1710044496
// 1741580496
func getValidTime() (string, string) {
	//currentTime := time.Nsow()
	//currentTimestamp := 1710044496
	//futureTime := currentTime.Add(15 * time.Minute)
	//futureTimestamp := futureTime.Unix()
	currentTimestampStr := strconv.FormatInt(1710044496, 16)
	futureTimestampStr := strconv.FormatInt(1820044496, 16)
	currentTimestampStrSupply := SupplyZero(currentTimestampStr, 64)
	futureTimestampStrSupply := SupplyZero(futureTimestampStr, 64)
	return currentTimestampStrSupply, futureTimestampStrSupply
}
func SupplyZero(prefix string, maxTo int) string {
	padding := maxTo - len(prefix)
	if padding > 0 {
		prefix = "0" + prefix
		prefix = fmt.Sprintf("%0*s", maxTo, prefix)
	}
	return prefix
}

func strategyGenerate(request *model.TryPayUserOpRequest) (*model.Strategy, error) {
	if forceStrategyId := request.ForceStrategyId; forceStrategyId != "" {
		//force strategy
		if strategy := GetStrategyById(forceStrategyId); strategy == nil {
			return nil, xerrors.Errorf("Not Support Strategy ID: [%w]", forceStrategyId)
		} else {
			return strategy, nil
		}
	}

	suitableStrategy, err := GetSuitableStrategy(request.ForceEntryPointAddress, request.ForceNetwork, request.ForceToken) //TODO
	if err != nil {
		return nil, err
	}
	if suitableStrategy == nil {
		return nil, xerrors.Errorf("Empty Strategies")
	}
	return suitableStrategy, nil
}
