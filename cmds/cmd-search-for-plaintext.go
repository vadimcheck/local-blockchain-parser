package cmds

import (
	"fmt"
	"os"
	"path/filepath"

	// "github.com/btcsuite/btcd/txscript"
	// "github.com/btcsuite/btcutil"

	"github.com/WikiLeaksFreedomForce/local-blockchain-parser/utils"
)

// var validTextChars = [65536]bool{}

// func init() {
// 	for _, r := range []rune{'\r', '\n', '\t'} {
// 		validTextChars[int(r)] = true
// 	}

// 	for i := 32; i < 127; i++ {
// 		validTextChars[i] = true
// 	}

// 	// for i := 128; i < 169; i++ {
// 	// 	validTextChars[i] = true
// 	// }
// }

func SearchForPlaintext(startBlock, endBlock uint64, inDir, outDir string) error {
	outSubdir := filepath.Join(".", outDir, "search-for-plaintext")

	err := os.MkdirAll(outSubdir, 0777)
	if err != nil {
		return err
	}

	// start a goroutine to log errors
	chErr := make(chan error)
	go func() {
		for err := range chErr {
			fmt.Println("error:", err)
		}
	}()

	// fill up our file semaphore so we can obtain tokens from it
	for i := 0; i < maxFiles; i++ {
		fileSemaphore <- true
	}

	// start a goroutine for each .dat file being parsed
	chDones := []chan bool{}
	for i := int(startBlock); i < int(endBlock)+1; i++ {
		chDone := make(chan bool)
		go searchPlaintextParseBlock(inDir, outSubdir, i, chErr, chDone)
		chDones = append(chDones, chDone)
	}

	// wait for all ops to complete
	for _, chDone := range chDones {
		<-chDone
	}

	// close error channel
	close(chErr)

	return nil
}

func searchPlaintextParseBlock(inDir string, outDir string, blockFileNum int, chErr chan error, chDone chan bool) {
	defer close(chDone)

	filename := fmt.Sprintf("blk%05d.dat", blockFileNum)
	fmt.Println("parsing block", filename)

	<-fileSemaphore
	blocks, err := utils.LoadBlockFile(filepath.Join(inDir, filename))
	fileSemaphore <- true
	if err != nil {
		chErr <- err
		return
	}

	outFile, err := createFile(filepath.Join(outDir, fmt.Sprintf("blk%05d-plaintext.txt", blockFileNum)))
	if err != nil {
		chErr <- err
		return
	}
	defer closeFile(outFile)

	for _, bl := range blocks {
		blockHash := bl.Hash().String()

		for _, tx := range bl.Transactions() {
			txHash := tx.Hash().String()

			for txinIdx, txin := range tx.MsgTx().TxIn {
				txt, isText := extractText(txin.SignatureScript)
				if !isText {
					continue
				}

				_, err := outFile.WriteString(fmt.Sprintf("%v,%v,%v,%v,%v\n", blockHash, txHash, "in", txinIdx, string(txt)))
				if err != nil {
					chErr <- err
					return
				}
			}

			for txoutIdx, txout := range tx.MsgTx().TxOut {
				txt, isText := extractText(txout.PkScript)
				if !isText {
					continue
				}

				_, err := outFile.WriteString(fmt.Sprintf("%v,%v,%v,%v,%v\n", blockHash, txHash, "out", txoutIdx, string(txt)))
				if err != nil {
					chErr <- err
					return
				}
			}

			parsedScriptData, err := concatNonOPHexTokensFromTxOuts(tx)
			if err != nil {
				chErr <- err
				return
			}

			parsedScriptText, isText := extractText(parsedScriptData)
			if err != nil {
				chErr <- err
				return
			}

			if isText && len(parsedScriptText) > 8 {
				fmt.Println(string(parsedScriptText))
				_, err := outFile.WriteString(fmt.Sprintf("%v,%v,%v,%v,%v\n", blockHash, txHash, "out", -1, string(parsedScriptText)))
				if err != nil {
					chErr <- err
					return
				}
			}
		}
	}

	if err != nil {
		chErr <- err
		return
	}
}

func extractText(bs []byte) ([]byte, bool) {
	start := 0

	for start < len(bs) {
		if isValidPlaintextByte(bs[start]) {
			break
		}
		start++
	}
	if start == len(bs) {
		return nil, false
	}

	end := start
	for end < len(bs) {
		if !isValidPlaintextByte(bs[end]) {
			break
		}
		end++
	}

	sublen := end - start + 1
	if sublen < 5 {
		return nil, false
	}

	substr := bs[start:end]
	return substr, true
}

func stripNonTextBytes(bs []byte) []byte {
	newBs := make([]byte, len(bs))
	newBsLen := 0
	for i := range bs {
		if isValidPlaintextByte(bs[i]) {
			newBs[newBsLen] = bs[i]
			newBsLen++
		}
	}

	if newBsLen == 0 {
		return nil
	}

	return newBs[0:newBsLen]
}

func isValidPlaintextByte(x byte) bool {
	switch x {
	case '\r', '\n', '\t', ' ':
		return true
	}

	i := int(rune(x))
	if i >= 32 && i < 127 {
		return true
	}

	return false
}