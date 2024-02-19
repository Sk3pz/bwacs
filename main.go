package main

import (
	"cloud.google.com/go/firestore"
	"context"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"fmt"
	"google.golang.org/api/option"
	"strings"
	"time"
)

/*
 * 1 call / 190 seconds (~3.16 minutes) = 10k / 31 day month w/ 7 hours of sleep time per day (17 total day hours)
 * 1 call / 268 seconds (~4.46 minutes) = 10k / 31 day month w/o sleep
 */

type Token struct {
	Uuid   string  `firestore:"uuid"`
	Token  string  `firestore:"token"`
	Lat    float64 `firestore:"latitude"`
	Long   float64 `firestore:"longitude"`
	Radius float64 `firestore:"radius"`
}

type Spot struct {
	Uuid     string `firestore:"uuid"`
	Hex      string `firestore:"hex"`
	Type     string `firestore:"type"`
	Callsign string `firestore:"callsign"`
}

func containsAircraft(aircraftList []Aircraft, targetAircraftHex string) bool {
	// Create a map to store the aircraft
	aircraftMap := make(map[string]bool)
	for _, a := range aircraftList {
		aircraftMap[a.Hex] = true
	}

	// Check if the target aircraft is in the map
	_, ok := aircraftMap[targetAircraftHex]
	return ok
}

func sleepUntilWake(wakeAt int) {
	// Get the current time
	now := time.Now()

	// Calculate the time of the next 6am
	var nextWake time.Time
	if now.Hour() < wakeAt {
		// It's currently before 6am, so the next 6am is today
		nextWake = time.Date(now.Year(), now.Month(), now.Day(), wakeAt, 0, 0, 0, now.Location())
	} else {
		// It's currently after 6am, so the next 6am is tomorrow
		nextWake = time.Date(now.Year(), now.Month(), now.Day()+1, wakeAt, 0, 0, 0, now.Location())
	}

	// Sleep until the next 6am
	sleepDuration := nextWake.Sub(now)
	time.Sleep(sleepDuration)
}

func main() {
	// Initialize Firebase Admin SDK
	opt := option.WithCredentialsFile("firebase_auth/auth.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		_ = fmt.Errorf("error initializing app: %v", err)
		return
	}

	// get config
	config, err := readConfig()
	if err != nil {
		_ = fmt.Errorf("error reading config: %v", err)
		return
	}

	// Initialize Firestore db_client
	dbClient, err := app.Firestore(context.Background())
	if err != nil {
		_ = fmt.Errorf("error initializing Firestore db_client: %v", err)
		return
	}
	defer func(dbClient *firestore.Client) {
		err := dbClient.Close()
		if err != nil {
			_ = fmt.Errorf("failed to close the client: %v", err)
		}
	}(dbClient)

	// Initialize FCM client
	fcmClient, err := app.Messaging(context.Background())
	if err != nil {
		_ = fmt.Errorf("error initializing FCM client: %v", err)
		return
	}

	// Create a ticker to send messages every delay
	//ticker := time.NewTicker(time.Duration(config.Delay * 1000000000))
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	countdown := config.Delay

	fmt.Printf(""+
		"         ,-.\n"+
		"        / \\  `.  __..-,O\n"+
		"       :   \\ --''_..-'.'\n"+
		"       |    . .-' `. '.\n"+
		"       :     .     .`.'\n"+
		"        \\     `.  /  ..\n"+
		"         \\      `.   ' .\n"+
		"          `,       `.   \\\n"+
		"         ,|,`.        `-.\\\n"+
		"        '.||  ``-...__..-`\n"+
		"         |  |\n"+
		"         |__|\n"+
		"         /||\\\n"+
		"        //||\\\\\n"+
		"       // || \\\\\n"+
		"    __//__||__\\\\__\n"+
		"   '--------------'\n"+
		"== Welcome to BWACS! ==\n"+
		"Delay: %d seconds\n"+
		"Sleep time: %d:00\n"+
		"Wake time: %d:00\n"+
		"=======================\n",
		config.Delay, config.SleepAt, config.WakeAt)

	for {
		// Check if the current time is between 11pm and 6am
		now := time.Now()
		if now.Hour() >= config.SleepAt || now.Hour() < config.WakeAt {
			fmt.Println("\rIt's Sleep Time! zzz")
			sleepUntilWake(config.WakeAt)
			fmt.Println("\rGood Morning!")
			continue
		}
		select {
		case <-ticker.C:
			if countdown == 0 {
				// countdown needs to be reset
				countdown = config.Delay
				fmt.Printf("\rScan!                          ")
			} else {
				// continue counting down
				countdown--
				if countdown > 99 {
					fmt.Printf("\r%d seconds left.", countdown)
				} else if countdown > 9 {
					fmt.Printf("\r0%d seconds left.", countdown)
				} else {
					fmt.Printf("\r00%d seconds left.", countdown)
				}
				continue
			}

			// get military aircraft in the area
			milAircraft := getMilitaryAircraft()
			if milAircraft == nil {
				fmt.Printf("\rDCon.                          ")
				continue
			}

			dbTokens, err := dbClient.Collection("tokens").Documents(context.Background()).GetAll()
			if err != nil {
				_ = fmt.Errorf("error getting tokens from Firestore: %v", err)
				continue
			}

			// Unwrap tokens into instances of the Token struct
			var tokens []Token
			for _, doc := range dbTokens {
				var t Token
				err := doc.DataTo(&t)
				if err != nil {
					_ = fmt.Errorf("error unwrapping token: %v", err)
					continue
				}
				tokens = append(tokens, t)
			}

			// Send a message to each token
			// todo: make asynchronous?
			for _, token := range tokens {
				// Send message to token if any military aircraft are in the airspace
				aircraft := filterAircraftInRadius(milAircraft, token.Lat, token.Long, token.Radius)

				// Get all previous spots for the given UUID
				spotsCollection := dbClient.Collection("userSpots")
				query := spotsCollection.Where("uuid", "==", token.Uuid)
				spotPlanes, err := query.Documents(context.Background()).GetAll()
				if err != nil {
					_ = fmt.Errorf("error getting user spots for user %s from Firestore: %v", token.Uuid, err)
					continue
				}
				var spots []Spot
				for _, doc := range spotPlanes {
					var s Spot
					err := doc.DataTo(&s)
					if err != nil {
						_ = fmt.Errorf("error unwrapping token: %v", err)
						continue
					}
					spots = append(spots, s)
				}

				// Remove planes that are no longer in the airspace
				for _, spot := range spots {
					if !containsAircraft(aircraft, spot.Hex) {
						// Delete the document from the userSpots collection where uuid = token.Uuid and hex = spot.Hex
						query := spotsCollection.Where("uuid", "==", token.Uuid).Where("hex", "==", spot.Hex)
						docs, err := query.Documents(context.Background()).GetAll()
						if err != nil {
							_ = fmt.Errorf("error getting spot for user %s from Firestore: %v", token.Uuid, err)
							continue
						}
						for _, doc := range docs {
							_, err := doc.Ref.Delete(context.Background())
							if err != nil {
								_ = fmt.Errorf("error deleting spot for user %s from Firestore: %v", token.Uuid, err)
								continue
							}
						}
					}
				}

			aircraftLoop:
				for _, a := range aircraft {
					// if the user has already been pinged about the current aircraft
					for _, ac := range spots {
						if ac.Hex == a.Hex {
							continue aircraftLoop
						}
					}

					// remove ground test objects
					if !strings.Contains(a.Type, "GND") {
						callsign := a.Callsign
						if callsign == "" {
							callsign = "??"
						}
						aType := a.Type
						if aType == "" {
							aType = "??"
						}

						// add the current plane to the database
						_, err = spotsCollection.NewDoc().Set(context.Background(), Spot{
							Uuid:     token.Uuid,
							Hex:      a.Hex,
							Callsign: a.Callsign,
							Type:     a.Type,
						})
						if err != nil {
							_ = fmt.Errorf("error adding new spot for user %s to Firestore: %v", token.Uuid, err)
						}

						_, err := fcmClient.Send(context.Background(), &messaging.Message{
							Token: token.Token,
							Notification: &messaging.Notification{
								Title: fmt.Sprintf("%s", callsign),
								Body:  fmt.Sprintf("%s", aType),
							},
						})
						if err != nil {
							_ = fmt.Errorf("error sending FCM message: %v", err)
							continue
						}
					}
				} // for range aircraft
				// fmt.Printf("Successfully informed user with UUID %s\n", token.Uuid)
			} // for range tokens

		} // select statement
	} // for
} // main
