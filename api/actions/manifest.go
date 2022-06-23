package actions

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	processTypeWeb = "web"
)

type Manifest struct {
	appRepo           CFAppRepository
	domainRepo        CFDomainRepository
	processRepo       CFProcessRepository
	routeRepo         CFRouteRepository
	defaultDomainName string
}

func NewManifest(appRepo CFAppRepository, domainRepo CFDomainRepository, processRepo CFProcessRepository, routeRepo CFRouteRepository, defaultDomainName string) *Manifest {
	return &Manifest{
		appRepo:           appRepo,
		domainRepo:        domainRepo,
		processRepo:       processRepo,
		routeRepo:         routeRepo,
		defaultDomainName: defaultDomainName,
	}
}

func (a *Manifest) Apply(ctx context.Context, authInfo authorization.Info, spaceGUID string, manifest payloads.Manifest) error {
	appInfo := manifest.Applications[0]
	exists := true
	appRecord, err := a.appRepo.GetAppByNameAndSpace(ctx, authInfo, appInfo.Name, spaceGUID)
	if err != nil {
		if errors.As(err, new(apierrors.NotFoundError)) {
			exists = false
		} else {
			return apierrors.ForbiddenAsNotFound(err)
		}
	}

	if appInfo.Memory != nil {
		found := false
		for _, process := range appInfo.Processes {
			if process.Type == processTypeWeb {
				found = true
			}
		}

		if !found {
			appInfo.Processes = append(appInfo.Processes, payloads.ManifestApplicationProcess{
				Type:   processTypeWeb,
				Memory: appInfo.Memory,
			})
		}
	}

	if exists {
		err = a.updateApp(ctx, authInfo, appRecord, appInfo)
	} else {
		appRecord, err = a.createApp(ctx, authInfo, spaceGUID, appInfo)
	}

	if err != nil {
		return err
	}

	if !appInfo.NoRoute {
		err = a.checkAndUpdateDefaultRoute(ctx, authInfo, appRecord, &appInfo)
		if err != nil {
			return err
		}

		err = a.checkAndUpdateRandomRoute(ctx, authInfo, appRecord, &appInfo)
		if err != nil {
			return err
		}

		return a.createOrUpdateRoutes(ctx, authInfo, appRecord, appInfo.Routes)
	}
	return nil
}

func (a *Manifest) hasRoutes(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, appInfo payloads.ManifestApplication) (bool, error) {
	if len(appInfo.Routes) > 0 || appInfo.DefaultRoute || appInfo.RandomRoute {
		return true, nil
	}
	existingRoutes, err := a.routeRepo.ListRoutesForApp(ctx, authInfo, appRecord.GUID, appRecord.SpaceGUID)
	if err != nil {
		return false, err
	}
	if len(existingRoutes) > 0 {
		return true, nil
	}

	return false, nil
}

// checkAndUpdateDefaultRoute may set the default route on the manifest when DefaultRoute is true
func (a *Manifest) checkAndUpdateDefaultRoute(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, appInfo *payloads.ManifestApplication) error {
	if !appInfo.DefaultRoute || len(appInfo.Routes) > 0 {
		return nil
	}

	existingRoutes, err := a.routeRepo.ListRoutesForApp(ctx, authInfo, appRecord.GUID, appRecord.SpaceGUID)
	if err != nil {
		return err
	}
	if len(existingRoutes) > 0 {
		return nil
	}

	_, err = a.domainRepo.GetDomainByName(ctx, authInfo, a.defaultDomainName)
	if err != nil {
		return apierrors.AsUnprocessableEntity(
			err,
			fmt.Sprintf("The configured default domain %q was not found", a.defaultDomainName),
			apierrors.NotFoundError{},
		)
	}
	defaultRouteString := appInfo.Name + "." + a.defaultDomainName
	defaultRoute := payloads.ManifestRoute{
		Route: &defaultRouteString,
	}
	// set the route field of the manifest with app-name . default domain
	appInfo.Routes = append(appInfo.Routes, defaultRoute)

	return nil
}

// checkAndUpdateRandomRoute may set the random route on the manifest when RandomRoute is true
func (a *Manifest) checkAndUpdateRandomRoute(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, appInfo *payloads.ManifestApplication) error {
	if !appInfo.RandomRoute || len(appInfo.Routes) > 0 {
		return nil
	}

	existingRoutes, err := a.routeRepo.ListRoutesForApp(ctx, authInfo, appRecord.GUID, appRecord.SpaceGUID)
	if err != nil {
		return err
	}
	if len(existingRoutes) > 0 {
		return nil
	}

	_, err = a.domainRepo.GetDomainByName(ctx, authInfo, a.defaultDomainName)
	if err != nil {
		return apierrors.AsUnprocessableEntity(
			err,
			fmt.Sprintf("The configured default domain %q was not found", a.defaultDomainName),
			apierrors.NotFoundError{},
		)
	}
	randomHostname := appInfo.Name + "-" + generateRandomRoute()
	routeString := randomHostname + "." + a.defaultDomainName
	randomRoute := payloads.ManifestRoute{
		Route: &routeString,
	}
	// set the route field of the manifest with a randomly generated name . default domain
	appInfo.Routes = append(appInfo.Routes, randomRoute)

	return nil
}

func (a *Manifest) updateApp(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, appInfo payloads.ManifestApplication) error {
	_, err := a.appRepo.CreateOrPatchAppEnvVars(ctx, authInfo, repositories.CreateOrPatchAppEnvVarsMessage{
		AppGUID:              appRecord.GUID,
		AppEtcdUID:           appRecord.EtcdUID,
		SpaceGUID:            appRecord.SpaceGUID,
		EnvironmentVariables: appInfo.Env,
	})
	if err != nil {
		return err
	}

	for _, processInfo := range appInfo.Processes {
		exists := true

		var process repositories.ProcessRecord
		process, err = a.processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, appRecord.GUID, processInfo.Type, appRecord.SpaceGUID)
		if err != nil {
			if errors.As(err, new(apierrors.NotFoundError)) {
				exists = false
			} else {
				return err
			}
		}

		if exists {
			_, err = a.processRepo.PatchProcess(ctx, authInfo, processInfo.ToProcessPatchMessage(process.GUID, appRecord.SpaceGUID))
		} else {
			hasRoutes, err := a.hasRoutes(ctx, authInfo, appRecord, appInfo)
			if err != nil {
				return err
			}

			err = a.processRepo.CreateProcess(ctx, authInfo, processInfo.ToProcessCreateMessage(appRecord.GUID, appRecord.SpaceGUID, hasRoutes))
		}
		if err != nil {
			return err
		}
	}

	return err
}

func (a *Manifest) createApp(ctx context.Context, authInfo authorization.Info, spaceGUID string, appInfo payloads.ManifestApplication) (repositories.AppRecord, error) {
	appRecord, err := a.appRepo.CreateApp(ctx, authInfo, appInfo.ToAppCreateMessage(spaceGUID))
	if err != nil {
		return appRecord, err
	}

	for _, processInfo := range appInfo.Processes {
		hasRoutes, err := a.hasRoutes(ctx, authInfo, appRecord, appInfo)
		if err != nil {
			return appRecord, err
		}
		message := processInfo.ToProcessCreateMessage(appRecord.GUID, spaceGUID, hasRoutes)
		err = a.processRepo.CreateProcess(ctx, authInfo, message)
		if err != nil {
			return appRecord, err
		}
	}

	return appRecord, nil
}

func (a *Manifest) createOrUpdateRoutes(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, routes []payloads.ManifestRoute) error {
	if len(routes) == 0 {
		return nil
	}

	routeString := *routes[0].Route
	hostName, domainName, path := splitRoute(routeString)

	domainRecord, err := a.domainRepo.GetDomainByName(ctx, authInfo, domainName)
	if err != nil {
		return fmt.Errorf("createOrUpdateRoutes: %w", err)
	}

	routeRecord, err := a.routeRepo.GetOrCreateRoute(
		ctx,
		authInfo,
		repositories.CreateRouteMessage{
			Host:            hostName,
			Path:            path,
			SpaceGUID:       appRecord.SpaceGUID,
			DomainGUID:      domainRecord.GUID,
			DomainNamespace: domainRecord.Namespace,
			DomainName:      domainRecord.Name,
		})
	if err != nil {
		return fmt.Errorf("createOrUpdateRoutes: %w", err)
	}

	routeRecord, err = a.routeRepo.AddDestinationsToRoute(ctx, authInfo, repositories.AddDestinationsToRouteMessage{
		RouteGUID:            routeRecord.GUID,
		SpaceGUID:            routeRecord.SpaceGUID,
		ExistingDestinations: routeRecord.Destinations,
		NewDestinations: []repositories.DestinationMessage{
			{
				AppGUID:     appRecord.GUID,
				ProcessType: processTypeWeb,
				Port:        8080,
				Protocol:    "http1",
			},
		},
	})

	return err
}

func splitRoute(route string) (string, string, string) {
	parts := strings.SplitN(route, ".", 2)
	hostName := parts[0]
	domainAndPath := parts[1]

	parts = strings.SplitN(domainAndPath, "/", 2)
	domain := parts[0]
	var path string
	if len(parts) > 1 {
		path = "/" + parts[1]
	}
	return hostName, domain, path
}

func generateRandomRoute() string {
	adjectives := getAdjectives()
	nouns := getNouns()
	rand.Seed(time.Now().Unix())
	suffix := string('a'+rune(rand.Intn(26))) + string('a'+rune(rand.Intn(26)))
	return adjectives[rand.Intn(len(adjectives))] + "-" + nouns[rand.Intn(len(nouns))] + "-" + suffix
}

func getAdjectives() []string {
	return []string{
		"accountable",
		"active",
		"agile",
		"anxious",
		"appreciative",
		"balanced",
		"bogus",
		"boisterous",
		"bold",
		"boring",
		"brash",
		"brave",
		"bright",
		"busy",
		"chatty",
		"cheerful",
		"chipper",
		"comedic",
		"courteous",
		"daring",
		"delightful",
		"egregious",
		"empathic",
		"excellent",
		"exhausted",
		"fantastic",
		"fearless",
		"fluent",
		"forgiving",
		"friendly",
		"funny",
		"generous",
		"grateful",
		"grouchy",
		"grumpy",
		"happy",
		"hilarious",
		"humble",
		"impressive",
		"insightful",
		"intelligent",
		"industrious",
		"interested",
		"kind",
		"lean",
		"mediating",
		"meditating",
		"nice",
		"noisy",
		"optimistic",
		"palm",
		"patient",
		"persistent",
		"proud",
		"quick",
		"quiet",
		"reflective",
		"relaxed",
		"reliable",
		"resplendent",
		"responsible",
		"responsive",
		"rested",
		"restless",
		"shiny",
		"shy",
		"silly",
		"sleepy",
		"smart",
		"spontaneous",
		"stellar",
		"surprised",
		"sweet",
		"talkative",
		"terrific",
		"thankful",
		"timely",
		"tired",
		"triumphant",
		"turbulent",
		"unexpected",
		"wacky",
		"wise",
		"zany",
	}
}

func getNouns() []string {
	return []string{
		"aardvark",
		"alligator",
		"antelope",
		"armadillo",
		"baboon",
		"badger",
		"bandicoot",
		"bat",
		"bear",
		"bilby",
		"bongo",
		"buffalo",
		"bushbuck",
		"camel",
		"capybara",
		"cassowary",
		"cat",
		"cheetah",
		"chimpanzee",
		"chipmunk",
		"civet",
		"crane",
		"crocodile",
		"dingo",
		"dog",
		"dugong",
		"duiker",
		"echidna",
		"eland",
		"elephant",
		"emu",
		"fossa",
		"fox",
		"gazelle",
		"gecko",
		"gelada",
		"genet",
		"gerenuk",
		"giraffe",
		"gnu",
		"gorilla",
		"grysbok",
		"guanaco",
		"hartebeest",
		"hedgehog",
		"hippopotamus",
		"hyena",
		"hyrax",
		"impala",
		"jackal",
		"jaguar",
		"kangaroo",
		"klipspringer",
		"koala",
		"kob",
		"kookaburra",
		"kudu",
		"lemur",
		"leopard",
		"lion",
		"lizard",
		"llama",
		"lynx",
		"manatee",
		"mandrill",
		"marmot",
		"meerkat",
		"mongoose",
		"mouse",
		"numbat",
		"nyala",
		"okapi",
		"oribi",
		"oryx",
		"ostrich",
		"otter",
		"panda",
		"pangolin",
		"panther",
		"parrot",
		"platypus",
		"porcupine",
		"possum",
		"puku",
		"quokka",
		"quoll",
		"rabbit",
		"ratel",
		"raven",
		"reedbuck",
		"rhinocerous",
		"roan",
		"sable",
		"serval",
		"shark",
		"sitatunga",
		"springhare",
		"squirrel",
		"swan",
		"tiger",
		"topi",
		"toucan",
		"turtle",
		"vicuna",
		"wallaby",
		"warthog",
		"waterbuck",
		"whale",
		"wildebeest",
		"wolf",
		"wolverine",
		"wombat",
		"zebra",
	}
}
