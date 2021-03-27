package mods

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidarkhanov/nanoid"
	"github.com/ngld/knossos/packages/api/client"
	"github.com/ngld/knossos/packages/libknossos/pkg/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type KnDep struct {
	ID       string
	Version  string
	Packages []string
}

type KnExe struct {
	File       string
	Label      string
	Properties struct {
		X64  bool
		SSE2 bool
		AVX  bool
		AVX2 bool
	}
}

type KnChecksum [2]string

type KnArchive struct {
	Checksum KnChecksum
	Filename string
	Dest     string
	URLs     []string
	FileSize int
}

type KnFile struct {
	Filename string
	Archive  string
	OrigName string
	Checksum KnChecksum
}

type KnPackage struct {
	Name         string
	Notes        string
	Status       string
	Environment  string
	Folder       string
	Dependencies []KnDep
	Executables  []KnExe
	Files        []KnArchive
	Filelist     []KnFile
	IsVp         bool
}

type KnMod struct {
	LocalPath     string
	Title         string
	Version       string
	Stability     string
	Description   string
	Logo          string
	Tile          string
	Banner        string
	ReleaseThread string `json:"release_thread"`
	Type          string
	ID            string
	Notes         string
	Folder        string
	FirstRelease  string `json:"first_release"`
	LastUpdate    string `json:"last_update"`
	Cmdline       string
	ModFlag       []string `json:"mod_flag"`
	Screenshots   []string
	Packages      []KnPackage
	Videos        []string
}

func convertPath(ctx context.Context, modPath, input string) *client.FileRef {
	if input == "" {
		return nil
	}

	ref := &client.FileRef{
		Fileid: "local_" + nanoid.New(),
		Urls:   []string{"file://" + filepath.ToSlash(filepath.Join(modPath, input))},
	}
	storage.ImportFile(ctx, ref)

	return ref
}

func convertChecksum(input KnChecksum) (*client.Checksum, error) {
	digest, err := hex.DecodeString(input[1])
	if err != nil {
		return nil, err
	}

	return &client.Checksum{
		Algo:   input[0],
		Digest: digest,
	}, nil
}

func ImportMods(ctx context.Context, modFiles []string) error {
	mod := KnMod{}
	return storage.ImportMods(ctx, func(ctx context.Context, importMod func(*client.Release) error) error {
		for _, modFile := range modFiles {
			data, err := ioutil.ReadFile(modFile)
			if err != nil {
				return err
			}

			err = json.Unmarshal(data, &mod)
			if err != nil {
				return err
			}

			modPath, err := filepath.Abs(filepath.Dir(modFile))
			if err != nil {
				return err
			}

			item := new(client.Release)
			item.Modid = mod.ID
			item.Version = mod.Version
			item.Title = mod.Title
			item.Description = mod.Description
			item.Teaser = convertPath(ctx, modPath, mod.Tile)
			item.Banner = convertPath(ctx, modPath, mod.Banner)
			item.ReleaseThread = mod.ReleaseThread
			item.Videos = mod.Videos
			item.Notes = mod.Notes
			item.Cmdline = mod.Cmdline

			if mod.FirstRelease != "" {
				releaseDate, err := time.Parse("2006-01-02", mod.FirstRelease)
				if err != nil {
					return err
				}

				item.Released = &timestamppb.Timestamp{
					Seconds: releaseDate.Unix(),
				}
			}

			if mod.LastUpdate != "" {
				updateDate, err := time.Parse("2006-01-02", mod.LastUpdate)
				if err != nil {
					return err
				}

				item.Updated = &timestamppb.Timestamp{
					Seconds: updateDate.Unix(),
				}
			}

			switch mod.Type {
			case "mod":
				item.Type = client.ModType_MOD
			case "tc":
				item.Type = client.ModType_TOTAL_CONVERSION
			case "engine":
				item.Type = client.ModType_ENGINE
			case "tool":
				item.Type = client.ModType_TOOL
			case "extension":
				item.Type = client.ModType_EXTENSION
			default:
				item.Type = client.ModType_MOD
			}

			if item.Type == client.ModType_ENGINE {
				switch mod.Stability {
				case "stable":
					item.Stability = client.ReleaseStability_STABLE
				case "rc":
					item.Stability = client.ReleaseStability_RC
				case "nightly":
					item.Stability = client.ReleaseStability_NIGHTLY
				}
			}

			for _, screen := range mod.Screenshots {
				item.Screenshots = append(item.Screenshots, convertPath(ctx, modPath, screen))
			}

			item.Packages = make([]*client.Package, len(mod.Packages))
			for pIdx, pkg := range mod.Packages {
				pbPkg := new(client.Package)
				pbPkg.Name = pkg.Name
				pbPkg.Folder = pkg.Folder
				pbPkg.Notes = pkg.Notes
				pbPkg.KnossosVp = pkg.IsVp

				switch pkg.Status {
				case "required":
					pbPkg.Type = client.PackageType_REQUIRED
				case "recommended":
					pbPkg.Type = client.PackageType_RECOMMENDED
				case "optional":
					pbPkg.Type = client.PackageType_OPTIONAL
				}

				// TODO: CpuSpec

				pbPkg.Dependencies = make([]*client.Dependency, len(pkg.Dependencies))
				for dIdx, dep := range pkg.Dependencies {
					pbDep := new(client.Dependency)
					pbDep.Modid = dep.ID
					pbDep.Constraint = dep.Version
					pbDep.Packages = dep.Packages
					pbPkg.Dependencies[dIdx] = pbDep
				}

				pbPkg.Archives = make([]*client.PackageArchive, len(pkg.Files))
				for aIdx, archive := range pkg.Files {
					pbArchive := new(client.PackageArchive)
					pbArchive.Id = archive.Filename
					pbArchive.Label = archive.Filename
					pbArchive.Destination = archive.Dest

					chk, err := convertChecksum(archive.Checksum)
					if err != nil {
						return err
					}
					pbArchive.Checksum = chk
					pbArchive.Filesize = uint64(archive.FileSize)
					pbArchive.Download = &client.FileRef{
						Fileid: "local_" + nanoid.New(),
						Urls:   archive.URLs,
					}

					pbPkg.Archives[aIdx] = pbArchive
				}

				pbPkg.Files = make([]*client.PackageFile, len(pkg.Filelist))
				for fIdx, file := range pkg.Filelist {
					pbFile := new(client.PackageFile)
					pbFile.Path = file.Filename
					pbFile.Archive = file.Archive
					pbFile.ArchivePath = file.OrigName

					chk, err := convertChecksum(file.Checksum)
					if err != nil {
						return err
					}
					pbFile.Checksum = chk
					pbPkg.Files[fIdx] = pbFile
				}

				pbPkg.Executables = make([]*client.EngineExecutable, len(pkg.Executables))
				for eIdx, exe := range pkg.Executables {
					pbExe := new(client.EngineExecutable)
					pbExe.Path = exe.File
					pbExe.Label = exe.Label

					prio := uint32(0)
					// See https://github.com/ngld/knossos/blob/1f60d925498c02d3db76a54d3ee20c31b75c5a21/knossos/repo.py#L35-L40
					if exe.Properties.X64 {
						prio += 50
					}
					if exe.Properties.AVX2 {
						prio += 3
					}
					if exe.Properties.AVX {
						prio += 2
					}
					if exe.Properties.SSE2 {
						prio++
					}
					pbExe.Priority = prio
					pbExe.Debug = strings.Contains(strings.ToLower(exe.Label), "debug")
					pbPkg.Executables[eIdx] = pbExe
				}

				item.Packages[pIdx] = pbPkg
			}

			err = importMod(item)
			if err != nil {
				return err
			}
		}

		return nil
	})
}