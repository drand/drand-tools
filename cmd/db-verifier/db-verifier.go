package main

import (
	"encoding/binary"
	json "github.com/nikkolasg/hexjson"
	"log"
	"time"

	"go.etcd.io/bbolt"
)

// beacon holds the randomness as well as the info to verify it.
type beacon struct {
	// Round is the round number this beacon is tied to
	Round uint64
	// Signature is the BLS deterministic signature over Round || PreviousRand
	Signature []byte
	// PreviousSig is the previous signature generated
	PreviousSig []byte `json:",omitempty"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// const baseFileName = "/home/florin/projects/drand/drand/b9s-db.bkp"
	// const baseFileName = "/home/florin/projects/drand/drand/def-bkp.bkp"
	const baseFileName = "/home/florin/projects/drand/drand/drand-testnet.bkp"
	// const baseFileName = "/home/florin/projects/drand/drand/drand-testnet-orig.bkp"
	// const baseFileName = "/home/florin/projects/drand/drand/drand-testnet-new-trimmed.bkp"

	bucketName := []byte("beacons")
	rows := 0
	started := time.Now()
	defer func() {
		finishedIn := time.Since(started)
		log.Printf("Finished verifying %s containing %d records in %s\n", baseFileName, rows, finishedIn)
	}()

	log.Printf("Start verifying %s\n", baseFileName)
	existingDB, err := bbolt.Open(baseFileName, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = existingDB.Close()
	}()

	err = existingDB.View(func(tx *bbolt.Tx) error {
		existingBucket := tx.Bucket(bucketName)

		var prevSig []byte
		var prevBeaconRound uint64
		rounds := make(map[string]uint64)
		err := existingBucket.ForEach(func(k, v []byte) error {
			rows++
			b := computeBeacon(k, v, prevSig)

			if prevBeaconRound > 0 &&
				b.Round-1 != prevBeaconRound {
				log.Printf("previous beacon round %d is farther behind than current round %d\n", prevBeaconRound, b.Round)
			}
			if previousRound, exists := rounds[string(b.Signature)]; exists {
				log.Printf("duplicate round signature found previous round: %d current round: %d!\n", previousRound, b.Round)
			}
			rounds[string(b.Signature)] = b.Round

			prevSig = v
			prevBeaconRound = b.Round

			return nil
		})

		if err != nil {
			log.Fatal(err)
		}
		log.Printf("last beacon round %d\n", prevBeaconRound)
		return nil
	})

	statsTime := func() {
		length := 0
		startedAt := time.Now()
		defer func() {
			finishedIn := time.Since(startedAt)
			log.Printf("db length %d - time processing: %s\n", length, finishedIn)
		}()

		err := existingDB.View(func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(bucketName)
			length = bucket.Stats().KeyN
			return nil
		})

		if err != nil {
			log.Fatal(err)
		}
	}

	statsTime()

	if err != nil {
		log.Fatal(err)
	}
}

func computeBeacon(k, v, prevSig []byte) beacon {
	var b beacon
	err := json.Unmarshal(v, &b)
	if err != nil {
		b = beacon{
			PreviousSig: prevSig,
			Round:       binary.BigEndian.Uint64(k),
			Signature:   v,
		}
	}

	return b
}
