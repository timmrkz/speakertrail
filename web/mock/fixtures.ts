// Fixture data for the dev-only mock API. Modelled on a real crawl of
// 24 Sep 2026, but every person, company and most event titles are invented.
// Days are offsets from today, so the calendar always has something on.

import type {
  AppearanceRole, Check, EventFormat, EventType, Fit, FetchMode, Health, PersonFit, ProfilePlatform,
  Review, Run, Setting, SourceKind, SourceStatus,
} from '../src/lib/api.ts'

// Berlin wall time to a UTC RFC 3339 string.
const berlinParts = new Intl.DateTimeFormat('en-GB', {
  timeZone: 'Europe/Berlin', year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
})
function offsetMinutes(utc: number): number {
  const p = Object.fromEntries(berlinParts.formatToParts(new Date(utc)).map((x) => [x.type, x.value]))
  const asUtc = Date.UTC(+p.year, +p.month - 1, +p.day, +p.hour, +p.minute, +p.second)
  return (asUtc - utc) / 60000
}
export function berlin(day: string, time: string): string {
  const [y, m, d] = day.split('-').map(Number)
  const [hh, mm] = time.split(':').map(Number)
  const guess = Date.UTC(y, m - 1, d, hh, mm)
  const utc = guess - offsetMinutes(guess) * 60000
  return new Date(utc).toISOString().replace('.000Z', 'Z')
}
export function todayBerlin(): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Berlin', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
}
export function addDays(day: string, n: number): string {
  const d = new Date(`${day}T12:00:00Z`)
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}
function hoursAgo(h: number): string {
  return new Date(Date.now() - h * 3600_000).toISOString().replace(/\.\d{3}Z$/, 'Z')
}

// People

type P = [name: string, role: AppearanceRole, affiliation: string]

interface EventSeed {
  day: number
  time: string
  end?: [day: number, time: string]
  city: string
  title: string
  venue: string
  address: string
  type: EventType
  organisers: string[]
  url: string
  price?: string
  format?: EventFormat
  fit?: Fit
  reason?: string
  sources: number[]
  people?: P[]
  seen?: number
}

export const EVENT_SEEDS: EventSeed[] = [
  // Past events, so some people have no upcoming appearance.
  { day: -21, time: '18:30', city: 'Münster', title: 'Start-up Night Münster: Fireside chat', venue: 'REACH Münster', address: 'Johann-Krane-Weg 21, 48149 Münster', type: 'talk', organisers: ['REACH Münster'], url: 'https://example.org/events/startup-night-muenster', sources: [9], reason: 'keep word: founder',
    people: [['Levin Straatmann', 'speaker', 'Co-founder, Sattelfest Bikes'], ['Hannah Ruppertz', 'moderator', 'REACH Münster']] },
  { day: -17, time: '10:00', city: 'Düsseldorf', title: 'Female Founders Summit', venue: 'Startplatz', address: 'Speditionstraße 15A, 40221 Düsseldorf', type: 'conference', organisers: ['Startplatz'], url: 'https://example.org/events/female-founders-summit', sources: [1], reason: 'keep word: founder',
    people: [['Wiebke Achterberg', 'speaker', 'Founder, Nachtruhe Sleep'], ['Selin Aksoy-Winter', 'panelist', 'Kiezkraft Labs']] },
  { day: -2, time: '09:00', city: 'Münster', title: 'Schüler Startup Gipfel', venue: 'Halle Münsterland', address: 'Albersloher Weg 32, 48155 Münster', type: 'conference', organisers: ['Startup Initiative NRW'], url: 'https://example.org/events/schueler-startup-gipfel', sources: [12], reason: 'keep word: gründer',
    people: [['Karim Belhadj-Moser', 'speaker', 'Co-founder, Hilfechat'], ['Theo Brandscheid', 'speaker', 'Founder, Ingwerwerk']] },

  // This week
  { day: 1, time: '08:30', end: [1, '10:00'], city: 'Düsseldorf', title: 'CreativeMornings: Play', venue: 'HSD Peter Behrens School of Arts', address: 'Münsterstraße 156, 40476 Düsseldorf', type: 'talk', organisers: ['CreativeMornings Düsseldorf'], url: 'https://example.org/events/creativemornings-play', price: 'Free', sources: [5], reason: 'keep word: maker',
    people: [['Jonas Feldhaus', 'speaker', 'Designer and founder, Kranwerk Studio'], ['Nele Kramps', 'host', 'CreativeMornings Düsseldorf']] },
  { day: 1, time: '09:00', end: [3, '16:00'], city: 'Hürth', title: 'Legal Hackathon Cologne', venue: 'Rheinland Legal Campus', address: 'Luxemburger Straße 449, 50354 Hürth', type: 'other', organisers: ['Legal Tech Lab Cologne'], url: 'https://example.org/events/legal-hackathon', price: 'Fully booked', sources: [14], reason: 'keep word: founder',
    people: [['Fabian Oettgen', 'speaker', 'Co-founder and CEO, Paragraphio'], ['Rana Haddadi', 'panelist', 'Legal Tech Lab Cologne'], ['Moritz Hellenbroich', 'panelist', 'Rheinmetrik'], ['Johanna Pütz-Albers', 'panelist', 'Ministry of Justice NRW'], ['Luca Mertens-Iwu', 'panelist', 'Hafenkapital']] },
  { day: 1, time: '17:00', city: 'Siegburg', title: 'Frauen-Zukunftstag', venue: 'KSI Siegburg', address: 'Bergstraße 26, 53721 Siegburg', type: 'conference', organisers: ['IHK Bonn/Rhein-Sieg'], url: 'https://example.org/events/frauen-zukunftstag', sources: [15], reason: 'keep word: gründer',
    people: [['Greta Ohlendorf', 'speaker', 'Moderator and keynote speaker'], ['Selin Aksoy-Winter', 'panelist', 'Kiezkraft Labs'], ['Anouk Terhorst', 'panelist', ''], ['Esra Kühlwetter', 'panelist', '']] },
  { day: 1, time: '10:00', end: [2, '18:00'], city: 'Münster', title: 'Münsterhack 2026', venue: 'Digital Hub münsterLAND', address: 'Hafenweg 16, 48155 Münster', type: 'other', organisers: ['Digital Hub münsterLAND'], url: 'https://example.org/events/muensterhack', sources: [9], reason: 'keep word: maker' },
  { day: 2, time: '12:45', city: 'Köln', title: 'Open day at the sports museum', venue: 'Deutsches Sport & Olympia Museum', address: 'Im Zollhafen 1, 50678 Köln', type: 'sport', organisers: ['Deutsches Sport & Olympia Museum'], url: 'https://example.org/events/open-day-sportmuseum', price: 'Free', sources: [18], reason: 'keep word: sport' },
  { day: 2, time: '10:00', city: 'Dortmund', title: 'Phoenix Run Club', venue: 'Phoenix-See', address: 'Hörder Hafenstraße, 44263 Dortmund', type: 'sport', organisers: ['Phoenix Run Club'], url: 'https://example.org/events/phoenix-run-club', price: 'Free', sources: [20], reason: 'keep word: sport',
    people: [['Tariq Behrendsen', 'host', 'Phoenix Run Club']] },
  { day: 2, time: '14:00', city: 'Online', title: 'Webinar: Funding programmes explained', venue: '', address: '', type: 'talk', format: 'online', organisers: ['Gründungsnetz NRW'], url: 'https://example.org/events/webinar-funding', fit: 'dropped', reason: 'drop word: webinar', sources: [11] },

  // Next week
  { day: 4, time: '18:00', end: [4, '21:30'], city: 'Köln', title: 'Rheinland Pitch #129', venue: 'Startplatz', address: 'Im Mediapark 5, 50670 Köln', type: 'pitch', organisers: ['Rheinland Pitch', 'Startplatz'], url: 'https://example.org/events/rheinland-pitch-129', price: 'Free', sources: [1, 2], reason: 'keep word: pitch',
    people: [['Lea Beispiel', 'pitch', 'Beispiel GmbH'], ['Malte Sieverding', 'pitch', 'Hafenbit'], ['Clara Nettekoven', 'pitch', 'Deichgrün Foods'], ['Levin Straatmann', 'host', 'Rheinland Pitch']] },
  { day: 4, time: '19:00', city: 'Köln', title: 'ProductTank Cologne: Switching careers', venue: 'Mediapark 8', address: 'Im Mediapark 8, 50670 Köln', type: 'panel', organisers: ['ProductTank Cologne'], url: 'https://example.org/events/producttank-careers', sources: [3], reason: 'keep word: meetup',
    people: [['Anouk Terhorst', 'panelist', 'Apothekenwerk'], ['Karim Belhadj-Moser', 'panelist', 'Beroweit'], ['Esra Kühlwetter', 'moderator', 'ProductTank Cologne']] },
  { day: 4, time: '18:00', city: 'Aachen', title: "Founders' Roundtable #15", venue: 'Collective Incubator', address: 'Jülicher Straße 72a, 52070 Aachen', type: 'pitch', organisers: ['Collective Incubator'], url: 'https://example.org/events/founders-roundtable-15', sources: [7], reason: 'keep word: founder',
    people: [['Theo Brandscheid', 'host', 'digitalHUB Aachen']] },
  { day: 4, time: '19:00', city: 'Köln', title: 'Reading night: Another class', venue: 'Literaturhaus', address: 'Großer Griechenmarkt 39, 50676 Köln', type: 'talk', organisers: ['Literaturhaus Köln'], url: 'https://example.org/events/reading-another-class', price: '12 €', sources: [16], fit: 'dropped', reason: 'no keep word',
    people: [['Vincent Laupichler', 'speaker', 'Author'], ['Marlene Oppitz', 'moderator', '']] },
  { day: 5, time: '12:30', city: 'Düsseldorf', title: 'VC-Stammtisch', venue: 'Medienhafen Lounge', address: 'Kaistraße 16, 40221 Düsseldorf', type: 'talk', organisers: ['VC-Stammtisch Düsseldorf'], url: 'https://example.org/events/vc-stammtisch', sources: [6], reason: 'keep word: founder' },
  { day: 5, time: '19:00', city: 'Köln', title: 'Female Business Meetup', venue: 'Stadtgarten', address: 'Venloer Straße 40, 50672 Köln', type: 'meetup', organisers: ['Golden Founders Köln'], url: 'https://example.org/events/female-business-meetup', price: '15 €', sources: [4], reason: 'keep word: meetup',
    people: [['Hannah Ruppertz', 'host', 'Golden Founders Köln']] },
  { day: 5, time: '10:00', city: 'Duisburg', title: 'Deep Dive: Smart and connected logistics', venue: 'startport', address: 'Philosophenweg 29, 47051 Duisburg', type: 'workshop', organisers: ['startport'], url: 'https://example.org/events/deep-dive-logistics', sources: [10], reason: 'keep word: founder' },
  { day: 5, time: '09:00', city: 'Essen', title: 'Sales training for founders', venue: 'Haus der Technik', address: 'Hollestraße 1, 45127 Essen', type: 'workshop', organisers: ['Haus der Technik'], url: 'https://example.org/events/sales-training', price: '390 €', fit: 'dropped', reason: 'drop word: sales', sources: [11] },
  { day: 6, time: '18:30', city: 'Köln', title: 'Gateway Sundowner', venue: 'InnoDom', address: 'Josef-Lammerting-Allee 25, 50933 Köln', type: 'talk', organisers: ['Gateway Uni Köln'], url: 'https://example.org/events/gateway-sundowner', price: 'Free', sources: [8], reason: 'keep word: gründer',
    people: [['Frieda Kallenbach', 'speaker', 'Co-founder, Lumenkraft']] },
  { day: 6, time: '18:30', city: 'Köln', title: 'AI Tinkerers Cologne: Live demos', venue: 'Südstadt Tech Campus', address: 'Domstraße 20, 50668 Köln', type: 'meetup', format: 'hybrid', organisers: ['AI Tinkerers Cologne'], url: 'https://example.org/events/ai-tinkerers-demos', price: 'By application', sources: [3, 13], reason: 'keep word: meetup',
    people: [['Konstantin Brühlmann', 'pitch', 'Co-founder, Schwarmfunk'], ['Aylin Sarıkaya-Brandt', 'pitch', 'Access e.V.'], ['Emre Toprakçı', 'pitch', 'Grünschnitt Analytics'], ['Ole Wiesendahl', 'pitch', 'Turmwerk'], ['Yusuf Dördelmann', 'pitch', 'Brückenbau Ventures'], ['Pia Schmelzer-Adu', 'pitch', '']] },
  { day: 6, time: '09:00', end: [8, '15:00'], city: 'Köln', title: 'Sports Games Symposium', venue: 'Deutsche Sporthochschule', address: 'Am Sportpark Müngersdorf 6, 50933 Köln', type: 'conference', organisers: ['DSHS Köln'], url: 'https://example.org/events/sports-games-symposium', sources: [18], reason: 'keep word: sport',
    people: [['Tariq Behrendsen', 'speaker', 'Sport psychologist'], ['Marlene Oppitz', 'speaker', ''], ['Niklas Scholven', 'speaker', ''], ['Ben Rautenstrauch', 'panelist', ''], ['Ida Voßkamp', 'panelist', ''], ['Luca Mertens-Iwu', 'panelist', ''], ['Svenja Hollweg', 'panelist', ''], ['Vincent Laupichler', 'panelist', ''], ['Johanna Pütz-Albers', 'panelist', '']] },
  { day: 7, time: '17:00', city: 'Köln', title: 'EdTech Meetup Cologne #7', venue: 'Holzmarkt Forum', address: 'Holzmarkt 2, 50676 Köln', type: 'meetup', organisers: ['KölnBusiness'], url: 'https://example.org/events/edtech-meetup-7', price: 'Free', sources: [17], reason: 'keep word: meetup',
    people: [['Moritz Hellenbroich', 'speaker', 'VR Lernwerk'], ['Paula Reifenrath', 'speaker', '']] },
  { day: 7, time: '16:00', city: 'Köln', title: 'Machwerktag', venue: 'machwerkhaus', address: 'Kalker Hauptstraße 55, 51103 Köln', type: 'other', organisers: ['machwerkhaus köln'], url: 'https://example.org/events/machwerktag', price: 'Free', sources: [19], reason: 'keep word: maker',
    people: [['Mira Obenauf', 'speaker', 'Ceramicist, Tassenwerk'], ['Svenja Hollweg', 'speaker', 'Ceramicist']] },
  { day: 7, time: '10:30', city: 'Neuss', title: 'Ideenfutter Expo', venue: 'Gare du Neuss', address: 'Hellersbergstraße 2, 41460 Neuss', type: 'conference', organisers: ['Foodhub NRW'], url: 'https://example.org/events/ideenfutter-expo', sources: [21], reason: 'keep word: founder',
    people: [['Deniz Karakurt-Lohmann', 'speaker', 'Zuckerwerk Rheinland'], ['Clara Nettekoven', 'speaker', 'Deichgrün Foods']] },
  { day: 7, time: '19:00', city: 'Köln', title: 'Game Creators Meetup #07', venue: 'Lost Level', address: 'Ehrenfeldgürtel 88, 50823 Köln', type: 'meetup', organisers: ['Game Creators Cologne'], url: 'https://example.org/events/game-creators-7', sources: [3], reason: 'keep word: meetup',
    people: [['Ida Voßkamp', 'host', 'Game Creators Cologne']] },
  { day: 7, time: '09:30', city: 'Düsseldorf', title: 'Start-up Breakfast', venue: 'Startplatz', address: 'Speditionstraße 15A, 40221 Düsseldorf', type: 'meetup', organisers: ['Startplatz'], url: 'https://example.org/events/startup-breakfast', price: 'Free', sources: [1], reason: 'keep word: founder' },
  { day: 8, time: '18:00', city: 'Bochum', title: 'Maker Evening Ruhr', venue: 'Unperfekthaus Bochum', address: 'Viktoriastraße 10, 44787 Bochum', type: 'workshop', organisers: ['Makerspace Ruhr'], url: 'https://example.org/events/maker-evening-ruhr', sources: [22], reason: 'keep word: maker',
    people: [['Niklas Scholven', 'host', 'Makerspace Ruhr']] },
  { day: 8, time: '09:00', city: 'Düsseldorf', title: 'Excel training for teams', venue: 'Bildungswerk Düsseldorf', address: 'Bismarckstraße 90, 40210 Düsseldorf', type: 'workshop', organisers: ['Bildungswerk Düsseldorf'], url: 'https://example.org/events/excel-training', fit: 'dropped', reason: 'drop word: training', sources: [11] },

  // Later this month
  { day: 9, time: '10:00', city: 'Essen', title: 'Ruhr Founders Run', venue: 'Grugapark', address: 'Virchowstraße 167a, 45147 Essen', type: 'sport', organisers: ['Ruhr Founders Club'], url: 'https://example.org/events/ruhr-founders-run', price: 'Free', sources: [20], reason: 'keep word: sport',
    people: [['Timo Hasselbach', 'host', 'Ruhr Founders Club']] },
  { day: 11, time: '18:30', city: 'Bonn', title: 'Bonn Pitch Night #41', venue: 'Digital Hub Bonn', address: 'Rheinwerkallee 6, 53227 Bonn', type: 'pitch', organisers: ['Digital Hub Bonn'], url: 'https://example.org/events/bonn-pitch-night-41', price: 'Free', sources: [23], reason: 'keep word: pitch',
    people: [['Mira Obenauf', 'pitch', 'Tassenwerk'], ['Emre Toprakçı', 'pitch', 'Grünschnitt Analytics'], ['Greta Ohlendorf', 'moderator', '']] },
  { day: 12, time: '19:00', city: 'Dortmund', title: 'Hardware Meetup Ruhr', venue: 'Dortmunder U', address: 'Leonie-Reygers-Terrasse, 44137 Dortmund', type: 'meetup', organisers: ['Hardware Ruhr'], url: 'https://example.org/events/hardware-meetup-ruhr', sources: [22], reason: 'keep word: meetup',
    people: [['Ben Rautenstrauch', 'speaker', 'Nordlicht Robotics']] },
  { day: 13, time: '18:00', city: 'Münster', title: 'Start-up Night Münster', venue: 'REACH Münster', address: 'Johann-Krane-Weg 21, 48149 Münster', type: 'pitch', organisers: ['REACH Münster'], url: 'https://example.org/events/startup-night-muenster-oct', sources: [9], reason: 'keep word: pitch',
    people: [['Levin Straatmann', 'speaker', 'Co-founder, Sattelfest Bikes'], ['Timo Hasselbach', 'moderator', 'Ruhr Founders Club']] },
  { day: 14, time: '08:30', city: 'Düsseldorf', title: 'Female Founders Breakfast', venue: 'Startplatz', address: 'Speditionstraße 15A, 40221 Düsseldorf', type: 'meetup', organisers: ['Startplatz'], url: 'https://example.org/events/female-founders-breakfast', sources: [1], reason: 'keep word: founder',
    people: [['Wiebke Achterberg', 'host', 'Founder, Nachtruhe Sleep']] },
  { day: 15, time: '14:00', city: 'Köln', title: 'Prototyping Lab', venue: 'Cologne Game Lab', address: 'Schanzenstraße 28, 51063 Köln', type: 'workshop', organisers: ['Cologne Game Lab'], url: 'https://example.org/events/prototyping-lab', price: '25 €', sources: [3], reason: 'keep word: maker' },
  { day: 18, time: '18:00', city: 'Köln', title: 'Rheinland Pitch #130', venue: 'Startplatz', address: 'Im Mediapark 5, 50670 Köln', type: 'pitch', organisers: ['Rheinland Pitch', 'Startplatz'], url: 'https://example.org/events/rheinland-pitch-130', price: 'Free', sources: [1, 2], reason: 'keep word: pitch' },
  { day: 20, time: '18:00', city: 'Aachen', title: 'Aachen Tech Talks: Batteries', venue: 'SuperC', address: 'Templergraben 57, 52062 Aachen', type: 'talk', organisers: ['Aachen Tech Talks'], url: 'https://example.org/events/aachen-tech-talks-batteries', sources: [7], reason: 'keep word: founder',
    people: [['Rana Haddadi', 'speaker', 'Zellwerk Energy'], ['Fabian Oettgen', 'moderator', 'Paragraphio']] },
  { day: 22, time: '11:00', end: [23, '18:00'], city: 'Essen', title: 'Zollverein Maker Market', venue: 'Zeche Zollverein', address: 'Gelsenkirchener Straße 181, 45309 Essen', type: 'other', organisers: ['Zollverein Makers'], url: 'https://example.org/events/zollverein-maker-market', price: '5 €', sources: [22], reason: 'keep word: maker' },
  { day: 25, time: '18:30', city: 'Düsseldorf', title: 'Panel: Scaling from NRW', venue: 'Kö-Bogen Forum', address: 'Königsallee 2, 40212 Düsseldorf', type: 'panel', organisers: ['Gründerszene Düsseldorf'], url: 'https://example.org/events/panel-scaling-nrw', sources: [6], reason: 'keep word: founder',
    people: [['Frieda Kallenbach', 'panelist', 'Lumenkraft'], ['Yusuf Dördelmann', 'panelist', 'Brückenbau Ventures'], ['Konstantin Brühlmann', 'moderator', 'Schwarmfunk']] },
  { day: 27, time: '09:30', city: 'Köln', title: 'Sport Innovation Summit', venue: 'RheinEnergieStadion', address: 'Aachener Straße 999, 50933 Köln', type: 'conference', organisers: ['SportsInnovation Köln'], url: 'https://example.org/events/sport-innovation-summit', price: '149 €', sources: [18], reason: 'keep word: sport',
    people: [['Tariq Behrendsen', 'speaker', 'Sport psychologist'], ['Pia Schmelzer-Adu', 'speaker', 'Athlete and founder, Sprintlabor']] },
  { day: 29, time: '19:00', city: 'Bielefeld', title: 'Founders Bar OWL', venue: 'Founders Foundation', address: 'Obernstraße 50, 33602 Bielefeld', type: 'meetup', organisers: ['Founders Foundation'], url: 'https://example.org/events/founders-bar-owl', sources: [24], reason: 'keep word: founder' },
]

interface PersonInfo {
  headline: string
  city: string
  fit: PersonFit
  profiles?: [ProfilePlatform, string, Review][]
  affiliations?: [string, string, boolean][]
  notes?: string
}

export const PERSON_INFO: Record<string, PersonInfo> = {
  'Lea Beispiel': { headline: 'Founder, Beispiel GmbH', city: 'Köln', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-lea', 'confirmed'], ['instagram', 'https://www.instagram.com/example.lea/', 'open']], affiliations: [['Beispiel GmbH', 'founder', true]] },
  'Jonas Feldhaus': { headline: 'Designer and founder, Kranwerk Studio', city: 'Düsseldorf', fit: 'maker', profiles: [['linkedin', 'https://www.linkedin.com/in/example-jonas', 'open'], ['instagram', 'https://www.instagram.com/example.kranwerk/', 'open']], affiliations: [['Kranwerk Studio', 'founder', true]] },
  'Fabian Oettgen': { headline: 'Co-founder and CEO, Paragraphio', city: 'Köln', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-fabian', 'confirmed'], ['instagram', 'https://www.instagram.com/example.fabian/', 'rejected']], affiliations: [['Paragraphio', 'founder', true]], notes: 'Good talker. Asked about legal tech in the Mittelstand.' },
  'Greta Ohlendorf': { headline: 'Moderator and keynote speaker', city: 'Bonn', fit: 'creator', profiles: [['linkedin', 'https://www.linkedin.com/in/example-greta', 'open'], ['instagram', 'https://www.instagram.com/example.greta/', 'open']] },
  'Konstantin Brühlmann': { headline: 'Co-founder, Schwarmfunk. Chapter lead AI Tinkerers Cologne', city: 'Köln', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-konstantin', 'confirmed']], affiliations: [['Schwarmfunk', 'founder', true], ['AI Tinkerers Cologne', 'organiser', true]] },
  'Frieda Kallenbach': { headline: 'Co-founder, Lumenkraft', city: 'Köln', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-frieda', 'open'], ['instagram', 'https://www.instagram.com/example.frieda/', 'open']], affiliations: [['Lumenkraft', 'founder', true]] },
  'Levin Straatmann': { headline: 'Co-founder, Sattelfest Bikes', city: 'Münster', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-levin', 'open']], affiliations: [['Sattelfest Bikes', 'founder', true], ['Rheinland Pitch', 'organiser', false]] },
  'Wiebke Achterberg': { headline: 'Founder, Nachtruhe Sleep', city: 'Düsseldorf', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-wiebke', 'open'], ['instagram', 'https://www.instagram.com/example.nachtruhe/', 'open']], affiliations: [['Nachtruhe Sleep', 'founder', true]] },
  'Karim Belhadj-Moser': { headline: 'Co-founder, Hilfechat', city: 'Münster', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-karim', 'open']], affiliations: [['Hilfechat', 'founder', true]] },
  'Theo Brandscheid': { headline: 'Founder, Ingwerwerk. Host at digitalHUB Aachen', city: 'Aachen', fit: 'founder', profiles: [['instagram', 'https://www.instagram.com/example.ingwerwerk/', 'open']], affiliations: [['Ingwerwerk', 'founder', true], ['digitalHUB Aachen', 'employee', true]] },
  'Mira Obenauf': { headline: 'Ceramicist, Tassenwerk', city: 'Köln', fit: 'maker', profiles: [['instagram', 'https://www.instagram.com/example.tassenwerk/', 'confirmed']], affiliations: [['Tassenwerk', 'founder', true]] },
  'Tariq Behrendsen': { headline: 'Sport psychologist. Runs the Phoenix Run Club', city: 'Dortmund', fit: 'athlete', affiliations: [['Phoenix Run Club', 'organiser', true]] },
  'Pia Schmelzer-Adu': { headline: 'Athlete and founder, Sprintlabor', city: 'Köln', fit: 'athlete', profiles: [['instagram', 'https://www.instagram.com/example.sprintlabor/', 'open']] },
  'Clara Nettekoven': { headline: 'Founder, Deichgrün Foods', city: 'Köln', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-clara', 'open']], affiliations: [['Deichgrün Foods', 'founder', true]] },
  'Malte Sieverding': { headline: 'CTO, Hafenbit', city: 'Köln', fit: 'founder', affiliations: [['Hafenbit', 'founder', true]] },
  'Hannah Ruppertz': { headline: 'Community lead, Golden Founders Köln', city: 'Köln', fit: 'creator', profiles: [['linkedin', 'https://www.linkedin.com/in/example-hannah', 'open']], affiliations: [['Golden Founders Köln', 'organiser', true]] },
  'Timo Hasselbach': { headline: 'Organiser, Ruhr Founders Club', city: 'Essen', fit: 'founder', affiliations: [['Ruhr Founders Club', 'organiser', true]] },
  'Moritz Hellenbroich': { headline: 'Founder, VR Lernwerk', city: 'Köln', fit: 'founder', profiles: [['linkedin', 'https://www.linkedin.com/in/example-moritz', 'open']] },
  'Emre Toprakçı': { headline: 'Data lead, Grünschnitt Analytics', city: 'Bonn', fit: 'founder' },
  'Rana Haddadi': { headline: 'Battery researcher, Zellwerk Energy', city: 'Aachen', fit: 'other' },
}

export interface MockSource {
  id: number
  name: string
  kind: SourceKind
  url: string | null
  query: string | null
  category: string
  city: string
  status: SourceStatus
  fetch_mode: FetchMode
  notes: string
  checks: number
  empty_checks_in_row: number
  points: number
  health: Health
  health_note: string
  discovered_from: string
  last_found: number | null
  last_mode: string
  last_error: string
  last_http: number
  checked_hours_ago: number | null
}

type S = [id: number, name: string, kind: SourceKind, url: string, city: string, status: SourceStatus, points: number, found: number | null, health: Health, note: string, from: string]

const SOURCE_ROWS: S[] = [
  [1, 'Startplatz events', 'listing', 'https://example.org/startplatz/events', 'Köln', 'active', 38, 7, 'ok', '', 'Imported from the brief'],
  [2, 'Rheinland Pitch', 'organiser_page', 'https://example.org/rheinland-pitch', 'Köln', 'active', 21, 2, 'ok', '', 'Imported from the brief'],
  [3, 'Meetup Köln tech groups', 'calendar_meetup', 'https://example.org/meetup/koeln-tech', 'Köln', 'active', 44, 12, 'ok', '', 'Imported from the brief'],
  [4, 'Eventbrite Köln business', 'calendar_eventbrite', 'https://example.org/eventbrite/koeln-business', 'Köln', 'active', 9, 3, 'warning', 'Found fewer events than usual', 'Imported from the brief'],
  [5, 'CreativeMornings Düsseldorf', 'organiser_page', 'https://example.org/creativemornings/duesseldorf', 'Düsseldorf', 'active', 12, 1, 'ok', '', 'From the starting list: CreativeMornings'],
  [6, 'Gründerszene Düsseldorf calendar', 'listing', 'https://example.org/gruenderszene-dus', 'Düsseldorf', 'probation', 6, 2, 'ok', '', 'Web search: startup events Düsseldorf'],
  [7, 'aachen.digital events', 'listing', 'https://example.org/aachen-digital/events', 'Aachen', 'active', 17, 4, 'ok', '', 'Imported from the brief'],
  [8, 'Gateway Uni Köln', 'organiser_page', 'https://example.org/gateway-unikoeln/events', 'Köln', 'active', 8, 1, 'ok', '', 'Found on Startplatz events'],
  [9, 'REACH Münster', 'organiser_page', 'https://example.org/reach-muenster', 'Münster', 'probation', 5, 2, 'ok', '', 'Web search: Gründung Münster Events'],
  [10, 'startport Duisburg', 'listing', 'https://example.org/startport/events', 'Duisburg', 'probation', 3, 1, 'warning', 'Page needs JavaScript. Switched to the browser', 'Web search: logistics startup Duisburg'],
  [11, 'Gründungsnetz NRW', 'newsletter', 'https://example.org/gruendungsnetz/newsletter', '', 'active', 2, 5, 'ok', '', 'Imported from the brief'],
  [12, 'Startup Initiative NRW', 'listing', 'https://example.org/startup-initiative-nrw', 'Münster', 'candidate', 0, null, 'never', '', 'From the starting list: Schüler Startup Gipfel'],
  [13, 'AI Tinkerers Cologne', 'calendar_luma', 'https://example.org/luma/ai-tinkerers-cologne', 'Köln', 'active', 15, 1, 'ok', '', 'Found on Meetup Köln tech groups'],
  [14, 'Legal Tech Lab Cologne', 'organiser_page', 'https://example.org/legal-tech-lab', 'Köln', 'probation', 4, 1, 'ok', '', 'From the starting list: Legal Hackathon'],
  [15, 'IHK Bonn/Rhein-Sieg events', 'listing', 'https://example.org/ihk-bonn/events', 'Bonn', 'probation', 2, 1, 'error', 'Timed out after 30 s', 'Web search: IHK Bonn Veranstaltungen'],
  [16, 'Literaturhaus Köln', 'listing', 'https://example.org/literaturhaus-koeln/programm', 'Köln', 'retired', 0, 0, 'warning', '4 empty checks in a row', 'Web search: Lesungen Köln'],
  [17, 'KölnBusiness events', 'listing', 'https://example.org/koelnbusiness/events', 'Köln', 'active', 7, 2, 'ok', '', 'Imported from the brief'],
  [18, 'Sport in Köln', 'calendar_ical', 'https://example.org/sport-koeln/calendar.ics', 'Köln', 'manual', 10, 3, 'ok', '', 'Added by hand'],
  [19, 'machwerkhaus köln', 'organiser_page', 'https://example.org/machwerkhaus/programm', 'Köln', 'candidate', 0, null, 'never', '', 'Found on KölnBusiness events'],
  [20, 'Run clubs NRW', 'listing', 'https://example.org/runclubs-nrw', 'Dortmund', 'candidate', 0, 2, 'ok', '', 'Web search: Lauftreff Gründer NRW'],
  [21, 'Foodhub NRW news', 'organiser_page', 'https://example.org/foodhub-nrw/news', 'Neuss', 'candidate', 0, 1, 'ok', '', 'Web search: Food Startup NRW'],
  [22, 'Makerspace Ruhr', 'calendar_ical', 'https://example.org/makerspace-ruhr/cal.ics', 'Bochum', 'probation', 3, 3, 'ok', '', 'From the starting list: maker events Ruhr'],
  [23, 'Digital Hub Bonn', 'listing', 'https://example.org/digitalhub-bonn/events', 'Bonn', 'active', 11, 2, 'ok', '', 'Imported from the brief'],
  [24, 'Founders Foundation', 'listing', 'https://example.org/founders-foundation/events', 'Bielefeld', 'candidate', 0, null, 'error', 'HTTP 403. The site blocks the crawler', 'Web search: Gründer Events OWL'],
  [25, 'Old Köln startup blog', 'listing', 'https://example.org/old-blog/events', 'Köln', 'retired', 0, 0, 'error', 'HTTP 404. The page is gone', 'Imported from the brief'],
]

// Hours since the nightly run at 05:00 Berlin, `nights` runs back.
export function nightHoursAgo(nights = 0): number {
  const today = todayBerlin()
  const last = Date.parse(berlin(today, '05:00')) < Date.now() ? today : addDays(today, -1)
  return (Date.now() - Date.parse(berlin(addDays(last, -nights), '05:00'))) / 3600_000
}

export function buildSources(): MockSource[] {
  const lastRun = nightHoursAgo(0)
  const out: MockSource[] = SOURCE_ROWS.map(([id, name, kind, url, city, status, points, found, health, note, from]) => ({
    id, name, kind, url, query: null, category: kind === 'organiser_page' ? 'Venue and organiser' : 'Event listing', city, status,
    fetch_mode: id === 10 ? 'browser' : 'auto', notes: '', checks: found === null ? 0 : Math.max(1, (id * 7) % 23),
    empty_checks_in_row: status === 'retired' ? 4 : found === 0 ? 1 : 0, points, health, health_note: note, discovered_from: from,
    last_found: found, last_mode: id === 10 ? 'browser' : 'http',
    last_error: health === 'error' ? note : '', last_http: health === 'error' ? (note.includes('403') ? 403 : note.includes('404') ? 404 : 0) : 200,
    checked_hours_ago: found === null && health === 'never' ? null : lastRun - 0.05 - (id % 9) * 0.03 + (status === 'retired' ? 24 * 12 : 0),
  }))
  out.push({
    id: 26, name: 'Web search: Gründer Stammtisch NRW', kind: 'search_query', url: null, query: 'Gründer Stammtisch NRW', category: 'Search', city: '',
    status: 'candidate', fetch_mode: 'auto', notes: '', checks: 1, empty_checks_in_row: 0, points: 1, health: 'ok', health_note: '',
    discovered_from: 'Added by the discovery job', last_found: 4, last_mode: 'search', last_error: '', last_http: 200, checked_hours_ago: lastRun - 0.3,
  })
  return out
}

export function buildRuns(sources: MockSource[]): { runs: Run[]; checks: Map<number, Check[]> } {
  const runs: Run[] = []
  const checks = new Map<number, Check[]>()
  let checkId = 900
  let fetchId = 3100
  for (let i = 0; i < 14; i++) {
    const id = 40 - i
    const startH = nightHoursAgo(i)
    const started = hoursAgo(startH)
    const running = false
    const list: Check[] = []
    const pool = sources.filter((s) => s.status !== 'retired' || i % 7 === 0).filter((s) => s.health !== 'never' || i > 0)
    for (const s of pool) {
      const fail = s.health === 'error' && (i < 3 || s.id === 25)
      const found = fail ? 0 : Math.max(0, (s.last_found ?? 2) + ((i * s.id) % 3) - 1)
      list.push({
        id: checkId++, source: { id: s.id, name: s.name, url: s.url ?? '' }, mode: s.last_mode, http_status: fail ? s.last_http : 200,
        events_found: found, events_kept: Math.max(0, found - ((s.id + i) % 2)), people_found: found * ((s.id % 3) + 1) - (found ? 1 : 0),
        error: fail ? s.health_note : '', duration_ms: fail && s.last_http === 0 ? 30000 : 600 + ((s.id * 379 + i * 211) % 4200) + (s.last_mode === 'browser' ? 6000 : 0),
        checked_at: hoursAgo(startH - 0.02 * list.length), fetch_id: fail && s.last_http === 0 ? null : fetchId++,
      })
    }
    checks.set(id, list)
    runs.push({
      id, kind: 'nightly', started_at: started, finished_at: running ? null : hoursAgo(startH - 0.02 * list.length - 0.05),
      sources_checked: list.length, events_found: list.reduce((a, c) => a + c.events_found, 0),
      events_new: Math.max(0, 9 - i + ((i * 5) % 7)), people_new: Math.max(0, 14 - i + ((i * 3) % 9)),
      errors: list.filter((c) => c.error).length,
    })
  }
  return { runs, checks }
}

export function buildSettings(): Setting[] {
  const t = hoursAgo(24 * 9)
  const rows: [string, Setting['value'], string][] = [
    ['public_calendar', true, 'Show the public event calendar at the front page'],
    ['public_show_people', true, 'Name the people on stage in the public calendar'],
    ['region', ['Köln', 'Bonn', 'Düsseldorf', 'Essen', 'Dortmund', 'Duisburg', 'Bochum', 'Aachen', 'Münster', 'Wuppertal', 'Bielefeld', 'Neuss', 'Hürth', 'Siegburg'], 'Cities in NRW whose in-person events count'],
    ['collect_ahead_days', 30, 'How far ahead events are collected, in days'],
    ['post_window_days', 7, 'Which events go into a post, in days from the post date'],
    ['active_check_interval_days', 1, 'Days between checks of an active source. 1 means every run'],
    ['probation_weeks', 3, 'Weeks a source stays on probation before it becomes active or retired'],
    ['probation_checks', 3, 'Checks before a candidate becomes active or retired'],
    ['retire_after_empty_checks', 4, 'Empty checks in a row before a source retires'],
    ['retired_recheck_days', 28, 'Days between checks of a retired source'],
    ['new_candidates_per_run', 10, 'Candidate sources checked for the first time per run'],
    ['profile_recheck_days', 90, 'Days before a profile is looked at again'],
    ['scoring_window_weeks', 8, 'How far back points count, in weeks'],
    ['discovery_share', 0.2, 'Share of points passed to the source that discovered another'],
    ['points', { event: 1, person: 1, reaction: 3, reply: 5, yes: 10 }, 'Points per result'],
    ['events_per_post', { min: 6, max: 10 }, 'Size of a post, grouped by city'],
    ['keep_words', ['founder', 'gründer', 'pitch', 'meetup', 'sport', 'maker'], 'First filter that keeps an event'],
    ['drop_words', ['webinar', 'training', 'sales'], 'First filter that drops an event'],
    ['snapshot_retention_days', 30, 'Days raw pages and screenshots are kept'],
    ['crawl_time', '05:00', 'Time of the nightly run, Europe/Berlin'],
    ['draft_days', ['sunday', 'wednesday'], 'Days the next post is drafted'],
    ['site_request_interval_seconds', 5, 'Minimum seconds between two requests to the same website'],
    ['worker_concurrency', 4, 'Jobs a worker runs at the same time'],
    ['browser_pages', 2, 'Headless browser pages open at the same time'],
    ['tavily_monthly_searches', 1000, 'Web searches per month, discovery and profiles together'],
    ['claude_monthly_budget_eur', 4, 'Monthly cap for Claude API calls, in euros'],
    ['voice_guide', '', 'How Tim writes, used when drafting posts'],
  ]
  return rows.map(([key, value, description]) => ({ key, value, description, updated_at: t }))
}

export { hoursAgo }
