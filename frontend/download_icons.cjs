const fs = require('fs');
const https = require('https');

const brands = [
  "netflix", "spotify", "youtube", "icloud", "apple", "telegram",
  "disney", "notion", "figma", "canva", "dropbox",
  "xbox", "playstation", "twitch", "nordvpn", "expressvpn",
  "todoist", "linear", "slack", "zoom", "duolingo", "strava",
  "headspace", "github", "adobe"
];

const map = {
  "chatgpt": "openai",
  "googleone": "google",
  "hbomax": "hbo",
  "crunchyroll": "crunchyroll",
  "onepassword": "1password",
  "vkmusic": "vk",
  "kinopoisk": "yandex",
  "yandexplus": "yandex",
  "applemusic": "apple"
};

const dl = (brand, iconName) => {
  return new Promise((resolve) => {
    https.get(`https://cdn.simpleicons.org/${iconName}/white`, (res) => {
      if (res.statusCode === 200) {
        const file = fs.createWriteStream(`public/icons/${brand}.svg`);
        res.pipe(file);
        file.on('finish', () => resolve(true));
      } else {
        resolve(false);
      }
    }).on('error', () => resolve(false));
  });
};

(async () => {
  console.log("Downloading icons...");
  for (const brand of brands) await dl(brand, brand);
  for (const [brand, icon] of Object.entries(map)) await dl(brand, icon);
  console.log("Done");
})();
