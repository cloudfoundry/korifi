package manifest

import (
	"math/rand"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/payloads"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

type Normalizer struct {
	defaultDomainName string
}

func NewNormalizer(defaultDomainName string) Normalizer {
	return Normalizer{
		defaultDomainName: defaultDomainName,
	}
}

func (n Normalizer) Normalize(appInfo payloads.ManifestApplication, appState AppState) payloads.ManifestApplication {
	fixDeprecatedFields(&appInfo)
	processes := n.normalizeProcesses(appInfo, appState)
	routes := n.normalizeRoutes(appInfo, appState)

	return payloads.ManifestApplication{
		Name:       appInfo.Name,
		Env:        appInfo.Env,
		Buildpacks: appInfo.Buildpacks,
		Processes:  processes,
		Routes:     routes,
		NoRoute:    appInfo.NoRoute,
	}
}

func procValIfSet[T any](appVal, procVal *T) *T {
	if procVal == nil {
		return appVal
	}
	return procVal
}

func fixDeprecatedFields(appInfo *payloads.ManifestApplication) {
	if appInfo.DiskQuota == nil {
		//nolint:staticcheck
		appInfo.DiskQuota = appInfo.AltDiskQuota
	}

	for i := range appInfo.Processes {
		if appInfo.Processes[i].DiskQuota == nil {
			//nolint:staticcheck
			appInfo.Processes[i].DiskQuota = appInfo.Processes[i].AltDiskQuota
		}
	}
}

func (n Normalizer) normalizeProcesses(appInfo payloads.ManifestApplication, appState AppState) []payloads.ManifestApplicationProcess {
	processes := appInfo.Processes

	var webProc *payloads.ManifestApplicationProcess
	for i, p := range processes {
		if p.Type == korifiv1alpha1.ProcessTypeWeb {
			webProc = &processes[i]
			break
		}
	}

	if appInfo.Memory != nil || appInfo.DiskQuota != nil || appInfo.Instances != nil || appInfo.Command != nil {
		if webProc == nil {
			processes = append(processes, payloads.ManifestApplicationProcess{Type: korifiv1alpha1.ProcessTypeWeb})
			webProc = &processes[len(processes)-1]
		}

		webProc.Memory = procValIfSet(appInfo.Memory, webProc.Memory)
		webProc.DiskQuota = procValIfSet(appInfo.DiskQuota, webProc.DiskQuota)
		webProc.Instances = procValIfSet(appInfo.Instances, webProc.Instances)
		webProc.Command = procValIfSet(appInfo.Command, webProc.Command)
	}

	return processes
}

func (n Normalizer) normalizeRoutes(appInfo payloads.ManifestApplication, appState AppState) []payloads.ManifestRoute {
	if appInfo.NoRoute {
		return nil
	}

	routes := appInfo.Routes

	needsAutomaticRoutes := len(routes) == 0 && len(appState.Routes) == 0
	if needsAutomaticRoutes {
		if appInfo.DefaultRoute {
			routes = append(routes, n.configureDefaultRoute(appInfo.Name))
		}

		if appInfo.RandomRoute {
			routes = append(routes, n.configureRandomRoute(appInfo.Name))
		}
	}

	return routes
}

func (n Normalizer) configureDefaultRoute(appName string) payloads.ManifestRoute {
	defaultRouteString := appName + "." + n.defaultDomainName
	return payloads.ManifestRoute{
		Route: &defaultRouteString,
	}
}

func (n Normalizer) configureRandomRoute(appName string) payloads.ManifestRoute {
	randomHostname := appName + "-" + generateRandomRoute()
	routeString := randomHostname + "." + n.defaultDomainName
	return payloads.ManifestRoute{
		Route: &routeString,
	}
}

func generateRandomRoute() string {
	rand.Seed(time.Now().Unix())
	suffix := string('a'+rune(rand.Intn(26))) + string('a'+rune(rand.Intn(26)))
	return adjectives[rand.Intn(len(adjectives))] + "-" + nouns[rand.Intn(len(nouns))] + "-" + suffix
}

var adjectives = strings.Split("accountable,active,agile,anxious,appreciative,balanced,bogus,boisterous,bold,boring,brash,brave,bright,busy,chatty,cheerful,chipper,comedic,courteous,daring,delightful,egregious,empathic,excellent,exhausted,fantastic,fearless,fluent,forgiving,friendly,funny,generous,grateful,grouchy,grumpy,happy,hilarious,humble,impressive,insightful,intelligent,industrious,interested,kind,lean,mediating,meditating,nice,noisy,optimistic,palm,patient,persistent,proud,quick,quiet,reflective,relaxed,reliable,resplendent,responsible,responsive,rested,restless,shiny,shy,silly,sleepy,smart,spontaneous,stellar,surprised,sweet,talkative,terrific,thankful,timely,tired,triumphant,turbulent,unexpected,wacky,wise,zany", ",")

var nouns = strings.Split("aardvark,alligator,antelope,armadillo,baboon,badger,bandicoot,bat,bear,bilby,bongo,buffalo,bushbuck,camel,capybara,cassowary,cat,cheetah,chimpanzee,chipmunk,civet,crane,crocodile,dingo,dog,dugong,duiker,echidna,eland,elephant,emu,fossa,fox,gazelle,gecko,gelada,genet,gerenuk,giraffe,gnu,gorilla,grysbok,guanaco,hartebeest,hedgehog,hippopotamus,hyena,hyrax,impala,jackal,jaguar,kangaroo,klipspringer,koala,kob,kookaburra,kudu,lemur,leopard,lion,lizard,llama,lynx,manatee,mandrill,marmot,meerkat,mongoose,mouse,numbat,nyala,okapi,oribi,oryx,ostrich,otter,panda,pangolin,panther,parrot,platypus,porcupine,possum,puku,quokka,quoll,rabbit,ratel,raven,reedbuck,rhinocerous,roan,sable,serval,shark,sitatunga,springhare,squirrel,swan,tiger,topi,toucan,turtle,vicuna,wallaby,warthog,waterbuck,whale,wildebeest,wolf,wolverine,wombat,zebra", ",")
