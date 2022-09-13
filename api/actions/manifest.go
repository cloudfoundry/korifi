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
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)
const (
	portHealthCheckType = "port"
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
	appExists := true
	appRecord, err := a.appRepo.GetAppByNameAndSpace(ctx, authInfo, appInfo.Name, spaceGUID)
	if err != nil {
		if !errors.As(err, new(apierrors.NotFoundError)) {
			return apierrors.ForbiddenAsNotFound(err)
		}
		appExists = false
	}

	if appInfo.Memory != nil {
		appInfo.Processes = appendWebProcessIfMissing(appInfo)
	}

	if appExists {
		return a.applyToUpdateApp(ctx, authInfo, appRecord, appInfo)
	}

	return a.applyToCreateApp(ctx, authInfo, spaceGUID, appInfo)
}

func appendWebProcessIfMissing(appInfo payloads.ManifestApplication) []payloads.ManifestApplicationProcess {
	for _, process := range appInfo.Processes {
		if process.Type == korifiv1alpha1.ProcessTypeWeb {
			return appInfo.Processes
		}
	}

	return append(appInfo.Processes, payloads.ManifestApplicationProcess{
		Type:   korifiv1alpha1.ProcessTypeWeb,
		Memory: appInfo.Memory,
	})
}

func (a *Manifest) applyToCreateApp(ctx context.Context, authInfo authorization.Info, spaceGUID string, appInfo payloads.ManifestApplication) error {
	appRecord, err := a.createApp(ctx, authInfo, spaceGUID, appInfo)
	if err != nil {
		return err
	}

	return a.configureRoutes(ctx, authInfo, appInfo, appRecord, []repositories.RouteRecord{})
}

func (a *Manifest) createApp(ctx context.Context, authInfo authorization.Info, spaceGUID string, appInfo payloads.ManifestApplication) (repositories.AppRecord, error) {
	appRecord, err := a.appRepo.CreateApp(ctx, authInfo, appInfo.ToAppCreateMessage(spaceGUID))
	if err != nil {
		return repositories.AppRecord{}, err
	}

	for index := range appInfo.Processes {
		processInfo := appInfo.Processes[index]
		err = a.createProcess(ctx, authInfo, appInfo, processInfo, appRecord, []repositories.RouteRecord{})
		if err != nil {
			return repositories.AppRecord{}, err
		}
	}

	return appRecord, nil
}

func (a *Manifest) createProcess(
	ctx context.Context,
	authInfo authorization.Info,
	appInfo payloads.ManifestApplication,
	processInfo payloads.ManifestApplicationProcess,
	appRecord repositories.AppRecord,
	existingAppRoutes []repositories.RouteRecord,
) error {
	if processInfo.Type == processTypeWeb && processInfo.HealthCheckType == nil {
		processInfo.HealthCheckType = computeDefaultWebProcessHealthCheckType(appInfo, existingAppRoutes)
	}

	message := processInfo.ToProcessCreateMessage(appRecord.GUID, appRecord.SpaceGUID)
	return a.processRepo.CreateProcess(ctx, authInfo, message)
}

func (a *Manifest) applyToUpdateApp(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, appInfo payloads.ManifestApplication) error {
	existingAppRoutes, err := a.routeRepo.ListRoutesForApp(ctx, authInfo, appRecord.GUID, appRecord.SpaceGUID)
	if err != nil {
		return err
	}
	err = a.updateApp(ctx, authInfo, appRecord, appInfo, existingAppRoutes)
	if err != nil {
		return err
	}

	return a.configureRoutes(ctx, authInfo, appInfo, appRecord, existingAppRoutes)
}

func (a *Manifest) updateApp(ctx context.Context, authInfo authorization.Info, appRecord repositories.AppRecord, appInfo payloads.ManifestApplication, existingAppRoutes []repositories.RouteRecord) error {
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
		err = a.createOrPatchProcess(ctx, authInfo, appRecord, appInfo, processInfo, existingAppRoutes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Manifest) createOrPatchProcess(
	ctx context.Context,
	authInfo authorization.Info,
	appRecord repositories.AppRecord,
	appInfo payloads.ManifestApplication,
	processInfo payloads.ManifestApplicationProcess,
	existingAppRoutes []repositories.RouteRecord,
) error {
	process, err := a.processRepo.GetProcessByAppTypeAndSpace(ctx, authInfo, appRecord.GUID, processInfo.Type, appRecord.SpaceGUID)
	if err != nil {
		if errors.As(err, new(apierrors.NotFoundError)) {
			return a.createProcess(ctx, authInfo, appInfo, processInfo, appRecord, existingAppRoutes)
		}
		return err
	}

	_, err = a.processRepo.PatchProcess(ctx, authInfo, processInfo.ToProcessPatchMessage(process.GUID, appRecord.SpaceGUID))
	return err
}

func computeDefaultWebProcessHealthCheckType(appInfo payloads.ManifestApplication, existingAppRoutes []repositories.RouteRecord) *string {
	hasRoutes := len(appInfo.Routes) > 0 || appInfo.DefaultRoute || appInfo.RandomRoute || len(existingAppRoutes) > 0

	if hasRoutes {
		healthCheckType := portHealthCheckType
		return &healthCheckType
	}

	return nil
}

func (a *Manifest) configureRoutes(ctx context.Context, authInfo authorization.Info, appInfo payloads.ManifestApplication, appRecord repositories.AppRecord, existingAppRoutes []repositories.RouteRecord) error {
	if appInfo.NoRoute {
		return a.deleteAppRoutes(ctx, authInfo, existingAppRoutes)
	}

	err := a.ensureDefaultDomainConfigured(ctx, authInfo)
	if err != nil {
		return err
	}

	needsAutomaticRoutes := len(appInfo.Routes) == 0 && len(existingAppRoutes) == 0
	if needsAutomaticRoutes {
		if appInfo.DefaultRoute {
			appInfo.Routes = append(appInfo.Routes, a.configureDefaultRoute(existingAppRoutes, &appInfo))
		}

		if appInfo.RandomRoute {
			appInfo.Routes = append(appInfo.Routes, a.configureRandomRoute(existingAppRoutes, &appInfo))
		}
	}

	return a.createOrUpdateRoutes(ctx, authInfo, appRecord, appInfo.Routes)
}

func (a *Manifest) deleteAppRoutes(ctx context.Context, authInfo authorization.Info, existingAppRoutes []repositories.RouteRecord) error {
	for _, route := range existingAppRoutes {
		err := a.routeRepo.DeleteRoute(ctx, authInfo, repositories.DeleteRouteMessage{
			GUID:      route.GUID,
			SpaceGUID: route.SpaceGUID,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Manifest) ensureDefaultDomainConfigured(ctx context.Context, authInfo authorization.Info) error {
	_, err := a.domainRepo.GetDomainByName(ctx, authInfo, a.defaultDomainName)
	if err != nil {
		return apierrors.AsUnprocessableEntity(
			err,
			fmt.Sprintf("The configured default domain %q was not found", a.defaultDomainName),
			apierrors.NotFoundError{},
		)
	}

	return nil
}

func (a *Manifest) configureDefaultRoute(existingAppRoutes []repositories.RouteRecord, appInfo *payloads.ManifestApplication) payloads.ManifestRoute {
	defaultRouteString := appInfo.Name + "." + a.defaultDomainName
	return payloads.ManifestRoute{
		Route: &defaultRouteString,
	}
}

func (a *Manifest) configureRandomRoute(existingAppRoutes []repositories.RouteRecord, appInfo *payloads.ManifestApplication) payloads.ManifestRoute {
	randomHostname := appInfo.Name + "-" + generateRandomRoute()
	routeString := randomHostname + "." + a.defaultDomainName
	return payloads.ManifestRoute{
		Route: &routeString,
	}
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
				ProcessType: korifiv1alpha1.ProcessTypeWeb,
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
