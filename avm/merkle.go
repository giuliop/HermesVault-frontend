package avm

import (
	"bytes"
	"fmt"

	"github.com/giuliop/HermesVault-frontend/config"
	"github.com/giuliop/HermesVault-frontend/db"
)

// getRoot returns the Merkle root from the database
func getRoot(leafIndex uint64) ([]byte, error) {
	root, leaf_count, err := db.GetRoot()
	if err != nil {
		return nil, fmt.Errorf("error getting root: %v", err)
	}
	if leafIndex >= leaf_count {
		return nil, fmt.Errorf("leaf index not in tree")
	}
	return root, nil
}

// createMerkleProof returns the Merkle proof for the leaf at the given index.
// The proof is a path that starts with the leaf value (not hashed)
// and includes the sibling hashes up to but excluding the root.
// It checks the validity of the proof against the provided root
func createMerkleProof(leafValue []byte, leafIndex uint64, root []byte) ([][]byte, error) {
	depth := config.MerkleTreeLevels
	proof := make([][]byte, 1, depth+1)
	proof[0] = leafValue

	// We need to decide whether we are left and add the right sibling to
	// the proof, or we are right and add the left sibling to the proof.
	// We can do this by checking the last bit of leaf index:
	// if it's 0, we are left, if it's 1, we are right.
	// We rigth shift the index to check the next bit in the next iteration.
	currentLevel, err := db.GetAllLeavesCommitments()
	if err != nil {
		return nil, fmt.Errorf("error getting all leaf commitments: %v", err)
	}
	if !bytes.Equal(config.Hash(leafValue), currentLevel[leafIndex]) {
		return nil, fmt.Errorf("leaf commitment mismatch")
	}
	if len(currentLevel)%2 == 1 {
		currentLevel = append(currentLevel, App.TreeConfig.ZeroHashes[0])
	}
	nextLevel := make([][]byte, (len(currentLevel)+1)/2)
	for i := 0; i < depth; i++ {
		if leafIndex&1 == 0 {
			proof = append(proof, currentLevel[leafIndex+1])
		} else {
			proof = append(proof, currentLevel[leafIndex-1])
		}

		for j := 0; j < len(currentLevel); j += 2 {
			nextLevel[j/2] = config.Hash(currentLevel[j], currentLevel[j+1])
		}
		if len(nextLevel)%2 == 1 {
			nextLevel = append(nextLevel, App.TreeConfig.ZeroHashes[i+1])
		}

		currentLevel = nextLevel
		nextLevel = nextLevel[:len(nextLevel)/2]
		leafIndex >>= 1
	}
	// check if the root for the proof is the same as the supplied root
	if len(nextLevel) != 1 || !bytes.Equal(nextLevel[0], root) {
		return nil, fmt.Errorf("root mismatch")
	}

	return proof, nil
}
