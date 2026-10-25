//go:build android

package tags

import (
	"strconv"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/process"
	"github.com/safing/portmaster/service/profile"
)

func init() {
	err := process.RegisterTagHandler(new(AndroidHandler))
	if err != nil {
		panic(err)
	}
}

const (
	androidName = "Android"
)

// AndroidHandler handles Android apps, which are identified by their UID and
// named by their package.
type AndroidHandler struct{}

// Name returns the tag handler name.
func (h *AndroidHandler) Name() string {
	return androidName
}

// TagDescriptions returns a list of all possible tags and their description
// of this handler.
func (h *AndroidHandler) TagDescriptions() []process.TagDescription {
	return []process.TagDescription{
		{
			ID:          profile.AndroidPackageTagKey,
			Name:        "Android Package",
			Description: "The package name of the Android app that owns the connection.",
		},
		{
			ID:          profile.AndroidUIDTagKey,
			Name:        "Android UID",
			Description: "The UID that the Android app runs as. Apps may be reassigned a UID after reinstalling.",
		},
		{
			ID:          profile.AndroidSharedUserTagKey,
			Name:        "Android Shared User",
			Description: "The sharedUserId of apps that are one firewall identity.",
		},
	}
}

// AddTags adds tags to the given process.
func (h *AndroidHandler) AddTags(p *process.Process) {
	// Special processes have a negative UID and carry no app.
	if p.UserID < 0 {
		return
	}

	info, err := aconn.GetAppInfo(p.UserID)
	if err != nil {
		log.Warningf("tags: failed to get android app info for uid %d: %s", p.UserID, err)
		return
	}

	if info.SharedUserID != "" {
		p.Tags = append(p.Tags, profile.Tag{
			Key:   profile.AndroidSharedUserTagKey,
			Value: info.SharedUserID,
		})
		return
	}

	p.Tags = append(p.Tags,
		profile.Tag{
			Key:   profile.AndroidPackageTagKey,
			Value: info.PackageName,
		},
		profile.Tag{
			Key:   profile.AndroidUIDTagKey,
			Value: strconv.Itoa(info.UID),
		},
	)
}

// CreateProfile creates a profile based on the tags of the process.
// Returns nil to skip.
func (h *AndroidHandler) CreateProfile(p *process.Process) *profile.Profile {
	// Fingerprint a shared identity on the sharedUserId name. The numeric UID
	// is reused by Android and must not carry rules to another app.
	tag, ok := p.GetTag(profile.AndroidSharedUserTagKey)
	if !ok {
		tag, ok = p.GetTag(profile.AndroidPackageTagKey)
	}
	if !ok {
		return nil
	}

	return profile.New(&profile.Profile{
		Source: profile.SourceLocal,
		Name:   p.Name,
		Fingerprints: []profile.Fingerprint{
			{
				Type:      profile.FingerprintTypeTagID,
				Key:       tag.Key,
				Operation: profile.FingerprintOperationEqualsID,
				Value:     tag.Value,
			},
		},
		// There is no readable binary to derive an icon or a name from; the
		// Android side supplies both.
		UsePresentationPath: false,
	})
}
