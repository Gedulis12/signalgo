package main

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/bridge"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-signal/database"
)

type Puppet struct {
	*database.Puppet

	bridge *SignalBridge
	log    zerolog.Logger

	MXID id.UserID

	customIntent *appservice.IntentAPI
	customUser   *User

	syncLock sync.Mutex
}

var userIDRegex *regexp.Regexp

var _ bridge.Ghost = (*Puppet)(nil)
var _ bridge.GhostWithProfile = (*Puppet)(nil)

// ** bridge.Ghost methods **
func (puppet *Puppet) GetMXID() id.UserID {
	return puppet.MXID
}

func (puppet *Puppet) DefaultIntent() *appservice.IntentAPI {
	return puppet.bridge.AS.Intent(puppet.MXID)
}

func (puppet *Puppet) CustomIntent() *appservice.IntentAPI {
	if puppet == nil {
		return nil
	}
	return puppet.customIntent
}

func (puppet *Puppet) IntentFor(portal *Portal) *appservice.IntentAPI {
	if puppet != nil {
		if puppet.customIntent == nil || portal.Key().ChatID == puppet.SignalID {
			return puppet.DefaultIntent()
		}
		return puppet.customIntent
	}
	return nil
}

// ** bridge.GhostWithProfile methods **
func (puppet *Puppet) GetDisplayname() string {
	return puppet.Name
}

func (puppet *Puppet) GetAvatarURL() id.ContentURI {
	return puppet.AvatarURL
}

// ** Puppet creation and fetching methods **
func (br *SignalBridge) NewPuppet(dbPuppet *database.Puppet) *Puppet {
	return &Puppet{
		Puppet: dbPuppet,
		bridge: br,
		log:    br.ZLog.With().Str("signal_user_id", dbPuppet.SignalID).Logger(),

		MXID: br.FormatPuppetMXID(dbPuppet.SignalID),
	}
}

func (br *SignalBridge) ParsePuppetMXID(mxid id.UserID) (string, bool) {
	if userIDRegex == nil {
		pattern := fmt.Sprintf(
			"^@%s:%s$",
			// The "SignalID" portion of the MXID is a (lowercase) UUID
			br.Config.Bridge.FormatUsername("([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})"),
			br.Config.Homeserver.Domain,
		)
		br.ZLog.Debug().Str("pattern", pattern).Msg("Compiling userIDRegex")

		userIDRegex = regexp.MustCompile(pattern)
	}

	match := userIDRegex.FindStringSubmatch(string(mxid))
	if len(match) == 2 {
		return match[1], true
	}

	return "", false
}

func (br *SignalBridge) GetPuppetByMXID(mxid id.UserID) *Puppet {
	signalID, ok := br.ParsePuppetMXID(mxid)
	if !ok {
		return nil
	}

	return br.GetPuppetBySignalID(signalID)
}

func (br *SignalBridge) GetPuppetBySignalID(id string) *Puppet {
	br.puppetsLock.Lock()
	defer br.puppetsLock.Unlock()

	if id == "" {
		br.ZLog.Warn().Msg("Trying to get puppet with empty signal_user_id")
		return nil
	}

	puppet, ok := br.puppets[id]
	if !ok {
		dbPuppet := br.DB.Puppet.GetBySignalID(id)
		if dbPuppet == nil {
			br.ZLog.Info().Str("signal_user_id", id).Msg("Puppet not found in database, creating new entry")
			dbPuppet = br.DB.Puppet.New()
			dbPuppet.SignalID = id
			//dbPuppet.Number =
			err := dbPuppet.Insert()
			if err != nil {
				br.ZLog.Error().Err(err).Str("signal_user_id", id).Msg("Error creating new puppet")
				return nil
			}
		}
		puppet = br.NewPuppet(dbPuppet)
		br.puppets[puppet.SignalID] = puppet
		if puppet.CustomMXID != "" {
			br.puppetsByCustomMXID[puppet.CustomMXID] = puppet
		}
		if puppet.Number != nil {
			br.puppetsByNumber[*puppet.Number] = puppet
		}
	}
	return puppet
}

func (br *SignalBridge) GetPuppetByNumber(number string) *Puppet {
	br.puppetsLock.Lock()
	defer br.puppetsLock.Unlock()

	puppet, ok := br.puppetsByNumber[number]
	if !ok {
		dbPuppet := br.DB.Puppet.GetByNumber(number)
		if dbPuppet == nil {
			return nil
		}

		puppet = br.NewPuppet(dbPuppet)
		br.puppets[puppet.SignalID] = puppet
		if puppet.CustomMXID != "" {
			br.puppetsByCustomMXID[puppet.CustomMXID] = puppet
		}
		if puppet.Number != nil {
			br.puppetsByNumber[*puppet.Number] = puppet
		}
	}
	return puppet
}

func (br *SignalBridge) GetPuppetByCustomMXID(mxid id.UserID) *Puppet {
	br.puppetsLock.Lock()
	defer br.puppetsLock.Unlock()

	puppet, ok := br.puppetsByCustomMXID[mxid]
	if !ok {
		dbPuppet := br.DB.Puppet.GetByCustomMXID(mxid)
		if dbPuppet == nil {
			return nil
		}

		puppet = br.NewPuppet(dbPuppet)
		br.puppets[puppet.SignalID] = puppet
		br.puppetsByCustomMXID[puppet.CustomMXID] = puppet
		if puppet.Number != nil {
			br.puppetsByNumber[*puppet.Number] = puppet
		}
	}
	return puppet
}

func (br *SignalBridge) GetAllPuppetsWithCustomMXID() []*Puppet {
	puppets, err := br.DB.Puppet.GetAllWithCustomMXID()
	if err != nil {
		br.ZLog.Error().Err(err).Msg("Failed to get all puppets with custom MXID")
		return nil
	}
	return br.dbPuppetsToPuppets(puppets)
}

func (br *SignalBridge) FormatPuppetMXID(did string) id.UserID {
	return id.NewUserID(
		br.Config.Bridge.FormatUsername(did),
		br.Config.Homeserver.Domain,
	)
}

func (br *SignalBridge) dbPuppetsToPuppets(dbPuppets []*database.Puppet) []*Puppet {
	br.puppetsLock.Lock()
	defer br.puppetsLock.Unlock()

	output := make([]*Puppet, len(dbPuppets))
	for index, dbPuppet := range dbPuppets {
		if dbPuppet == nil {
			continue
		}
		puppet, ok := br.puppets[dbPuppet.SignalID]
		if !ok {
			puppet = br.NewPuppet(dbPuppet)
			br.puppets[dbPuppet.SignalID] = puppet
			if dbPuppet.Number != nil {
				br.puppetsByNumber[*dbPuppet.Number] = puppet
			}
			if dbPuppet.CustomMXID != "" {
				br.puppetsByCustomMXID[dbPuppet.CustomMXID] = puppet
			}
		}
		output[index] = puppet
	}
	return output
}
