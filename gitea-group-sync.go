package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/ldap.v3"
)

func AddUsersToTeam(apiKeys GiteaKeys, users []Account, team int) bool {

	for i := 0; i < len(users); i++ {

		fullusername := url.PathEscape(fmt.Sprintf("%s", users[i].Full_name))
		apiKeys.Command = "/api/v1/users/search?q=" + fullusername + "&access_token="
		foundUsers := RequestSearchResults(apiKeys)

		for j := 0; j < len(foundUsers.Data); j++ {

			if strings.EqualFold(users[i].Login, foundUsers.Data[j].Login) {
				apiKeys.Command = "/api/v1/teams/" + fmt.Sprintf("%d", team) + "/members/" + foundUsers.Data[j].Login + "?access_token="
				error := RequestPut(apiKeys)
				if len(error) > 0 {
					log.Println("Error (Team does not exist or Not Found User) :", parseJson(error).(map[string]interface{})["message"])
				}
			}
		}
	}
	return true
}

func DelUsersFromTeam(apiKeys GiteaKeys, Users []Account, team int) bool {

	for i := 0; i < len(Users); i++ {

		apiKeys.Command = "/api/v1/users/search?uid=" + fmt.Sprintf("%d", Users[i].Id) + "&access_token="

		foundUser := RequestSearchResults(apiKeys)

		apiKeys.Command = "/api/v1/teams/" + fmt.Sprintf("%d", team) + "/members/" + foundUser.Data[0].Login + "?access_token="
		RequestDel(apiKeys)
	}
	return true
}

func main() {

	mainJob() // First run for check settings

	var repTime string
	if len(os.Getenv("REP_TIME")) == 0 {

	} else {
		repTime = os.Getenv("REP_TIME")
	}

	c := cron.New()
	c.AddFunc(repTime, mainJob)
	c.Start()
	fmt.Println(c.Entries())
	for true {
		time.Sleep(100 * time.Second)
	}
}

// Config describes the settings of the application. This structure is used in the settings-import process
type Config struct {
	apiKeys                   GiteaKeys
	ldapUrl                   string
	ldapPort                  int
	ldapTls                   bool
	ldapBindDN                string
	ldapBindPassword          string
	ldapFilter                string
	ldapUserSearchBase        string
	repTime                   string
	ldapUserIdentityAttribute string
	ldapUserFullName          string
}

// This Function parses the enviroment for application specific variables and returns a Config struct.
// Used for setting all required settings in the application
func importEnvVars() Config {

	// Create temporary structs for creating the final config
	envConfig := Config{}
	envConfig.apiKeys = GiteaKeys{}

	// Start parsing env. Variables
	if len(os.Getenv("GITEA_TOKEN")) < 40 { // get on  https://[web_site_url]/user/settings/applications
		log.Println("GITEA_TOKEN is empty or invalid.")
	} else {
		envConfig.apiKeys.TokenKey = strings.Split(os.Getenv("GITEA_TOKEN"), ",")
	}

	if len(os.Getenv("GITEA_URL")) == 0 {
		log.Println("GITEA_URL is empty")
	} else {
		envConfig.apiKeys.BaseUrl = os.Getenv("GITEA_URL")
	}

	if len(os.Getenv("LDAP_URL")) == 0 {
		log.Println("LDAP_URL is empty")
	} else {
		envConfig.ldapUrl = os.Getenv("LDAP_URL")
	}

	if len(os.Getenv("LDAP_TLS_PORT")) > 0 {
		port, err := strconv.Atoi(os.Getenv("LDAP_TLS_PORT"))
		envConfig.ldapPort = port
		envConfig.ldapTls = true
		log.Printf("DialTLS:=%v:%d", envConfig.ldapUrl, envConfig.ldapPort)
		if err != nil {
			log.Println("LDAP_TLS_PORT is invalid.")
		}
	} else {
		if len(os.Getenv("LDAP_PORT")) > 0 {
			port, err := strconv.Atoi(os.Getenv("LDAP_PORT"))
			envConfig.ldapPort = port
			envConfig.ldapTls = false
			log.Printf("Dial:=%v:%d", envConfig.ldapUrl, envConfig.ldapPort)
			if err != nil {
				log.Println("LDAP_PORT is invalid.")
			}
		}
	}

	if len(os.Getenv("BIND_DN")) == 0 {
		log.Println("BIND_DN is empty")
	} else {
		envConfig.ldapBindDN = os.Getenv("BIND_DN")
	}

	if len(os.Getenv("BIND_PASSWORD")) == 0 {
		log.Println("BIND_PASSWORD is empty")
	} else {
		envConfig.ldapBindPassword = os.Getenv("BIND_PASSWORD")
	}

	if len(os.Getenv("LDAP_FILTER")) == 0 {
		log.Println("LDAP_FILTER is empty")
	} else {
		envConfig.ldapFilter = os.Getenv("LDAP_FILTER")
	}

	if len(os.Getenv("LDAP_USER_SEARCH_BASE")) == 0 {
		log.Println("LDAP_USER_SEARCH_BASE is empty")
	} else {
		envConfig.ldapUserSearchBase = os.Getenv("LDAP_USER_SEARCH_BASE")
	}

	if len(os.Getenv("LDAP_USER_IDENTITY_ATTRIBUTE")) == 0 {
		envConfig.ldapUserIdentityAttribute = "uid"
		log.Println("By default LDAP_USER_IDENTITY_ATTRIBUTE = 'uid'")
	} else {
		envConfig.ldapUserIdentityAttribute = os.Getenv("LDAP_USER_IDENTITY_ATTRIBUTE")
	}

	if len(os.Getenv("LDAP_USER_FULL_NAME")) == 0 {
		envConfig.ldapUserFullName = "sn" //change to cn if you need it
		log.Println("By default LDAP_USER_FULL_NAME = 'sn'")
	} else {
		envConfig.ldapUserFullName = os.Getenv("LDAP_USER_FULL_NAME")
	}

	return envConfig // return the config struct for use.
}

func mainJob() {

	//------------------------------
	//  Check and Set input settings
	//------------------------------

	log.Println("Fallback: Importing Settings from Enviroment Variables ")
	cfg := importEnvVars()

	// Prepare LDAP Connection
	var l *ldap.Conn
	var err error
	if cfg.ldapTls {
		l, err = ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", cfg.ldapUrl, cfg.ldapPort), &tls.Config{InsecureSkipVerify: true})
	} else {
		l, err = ldap.Dial("tcp", fmt.Sprintf("%s:%d", cfg.ldapUrl, cfg.ldapPort))
	}

	if err != nil {
		fmt.Println(err)
		fmt.Println("Please set the correct values for all specifics.")
		os.Exit(1)
	}
	defer l.Close()

	err = l.Bind(cfg.ldapBindDN, cfg.ldapBindPassword)
	if err != nil {
		log.Fatal(err)
	}
	page := 1
	cfg.apiKeys.BruteforceTokenKey = 0
	cfg.apiKeys.Command = "/api/v1/admin/orgs?page=" + fmt.Sprintf("%d", page) + "&limit=20&access_token=" // List all organizations
	organizationList := RequestOrganizationList(cfg.apiKeys)

	log.Printf("%d organizations were found on the server: %s", len(organizationList), cfg.apiKeys.BaseUrl)

	for 1 < len(organizationList) {

		for i := 0; i < len(organizationList); i++ {

			log.Println(organizationList)

			log.Printf("Begin an organization review: OrganizationName= %v, OrganizationId= %d \n", organizationList[i].Name, organizationList[i].Id)

			cfg.apiKeys.Command = "/api/v1/orgs/" + organizationList[i].Name + "/teams?access_token="
			teamList := RequestTeamList(cfg.apiKeys)
			log.Printf("%d teams were found in %s organization", len(teamList), organizationList[i].Name)
			log.Printf("Skip synchronization in the Owners team")
			cfg.apiKeys.BruteforceTokenKey = 0

			for j := 1; j < len(teamList); j++ {

				// preparing request to ldap server
				filter := fmt.Sprintf(cfg.ldapFilter, teamList[j].Name)
				searchRequest := ldap.NewSearchRequest(
					cfg.ldapUserSearchBase, // The base dn to search
					ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
					filter, // The filter to apply
					[]string{"cn", "uid", "mailPrimaryAddress, sn", cfg.ldapUserIdentityAttribute}, // A list attributes to retrieve
					nil,
				)
				// make request to ldap server
				sr, err := l.Search(searchRequest)
				if err != nil {
					log.Fatal(err)
				}
				AccountsLdap := make(map[string]Account)
				AccountsGitea := make(map[string]Account)
				var addUserToTeamList, delUserToTeamlist []Account
				if len(sr.Entries) > 0 {
					log.Printf("The LDAP %s has %d users corresponding to team %s", cfg.ldapUrl, len(sr.Entries), teamList[j].Name)
					for _, entry := range sr.Entries {

						AccountsLdap[entry.GetAttributeValue(cfg.ldapUserIdentityAttribute)] = Account{
							Full_name: entry.GetAttributeValue(cfg.ldapUserFullName),
							Login:     entry.GetAttributeValue(cfg.ldapUserIdentityAttribute),
						}
					}

					cfg.apiKeys.Command = "/api/v1/teams/" + fmt.Sprintf("%d", teamList[j].Id) + "/members?access_token="
					AccountsGitea, cfg.apiKeys.BruteforceTokenKey = RequestUsersList(cfg.apiKeys)
					log.Printf("The gitea %s has %d users corresponding to team %s Teamid=%d", cfg.apiKeys.BaseUrl, len(AccountsGitea), teamList[j].Name, teamList[j].Id)

					for k, v := range AccountsLdap {
						if AccountsGitea[k].Login != v.Login {
							addUserToTeamList = append(addUserToTeamList, v)
						}
					}
					log.Printf("can be added users list %v", addUserToTeamList)
					AddUsersToTeam(cfg.apiKeys, addUserToTeamList, teamList[j].Id)

					for k, v := range AccountsGitea {
						if AccountsLdap[k].Login != v.Login {
							delUserToTeamlist = append(delUserToTeamlist, v)
						}
					}
					log.Printf("must be del users list %v", delUserToTeamlist)
					DelUsersFromTeam(cfg.apiKeys, delUserToTeamlist, teamList[j].Id)

				} else {
					log.Printf("The LDAP %s not found users corresponding to team %s", cfg.ldapUrl, teamList[j].Name)
				}
			}
		}

		page++
		cfg.apiKeys.BruteforceTokenKey = 0
		cfg.apiKeys.Command = "/api/v1/admin/orgs?page=" + fmt.Sprintf("%d", page) + "&limit=20&access_token=" // List all organizations
		organizationList = RequestOrganizationList(cfg.apiKeys)
		log.Printf("%d organizations were found on the server: %s", len(organizationList), cfg.apiKeys.BaseUrl)
	}
}
