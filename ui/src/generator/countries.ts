// Countries for the generator: the codes that a profile needs (country, usage location, calling code, language), and made-up name lists.
// Everything is a start that the person can change in the wizard. The names are common first names and surnames, written by hand for
// fake people: no real person is meant, and no list is complete.

export interface Country {
  code: string; // ISO 3166 alpha-2: countryLetterCode and usageLocation
  name: string;
  calling: string; // the digits after + in a phone number
  language: string; // preferredLanguage of the tenant, for example nb
  locale: string; // preferredLanguage of a user, for example nb-NO
  languageName: string;
}

export const countries: Country[] = [
  { code: "AT", name: "Austria", calling: "43", language: "de", locale: "de-AT", languageName: "German" },
  { code: "BE", name: "Belgium", calling: "32", language: "nl", locale: "nl-BE", languageName: "Dutch" },
  { code: "CA", name: "Canada", calling: "1", language: "en", locale: "en-CA", languageName: "English" },
  { code: "CH", name: "Switzerland", calling: "41", language: "de", locale: "de-CH", languageName: "German" },
  { code: "CZ", name: "Czechia", calling: "420", language: "cs", locale: "cs-CZ", languageName: "Czech" },
  { code: "DE", name: "Germany", calling: "49", language: "de", locale: "de-DE", languageName: "German" },
  { code: "DK", name: "Denmark", calling: "45", language: "da", locale: "da-DK", languageName: "Danish" },
  { code: "EE", name: "Estonia", calling: "372", language: "et", locale: "et-EE", languageName: "Estonian" },
  { code: "ES", name: "Spain", calling: "34", language: "es", locale: "es-ES", languageName: "Spanish" },
  { code: "FI", name: "Finland", calling: "358", language: "fi", locale: "fi-FI", languageName: "Finnish" },
  { code: "FR", name: "France", calling: "33", language: "fr", locale: "fr-FR", languageName: "French" },
  { code: "GB", name: "United Kingdom", calling: "44", language: "en", locale: "en-GB", languageName: "English" },
  { code: "IE", name: "Ireland", calling: "353", language: "en", locale: "en-IE", languageName: "English" },
  { code: "IS", name: "Iceland", calling: "354", language: "is", locale: "is-IS", languageName: "Icelandic" },
  { code: "IT", name: "Italy", calling: "39", language: "it", locale: "it-IT", languageName: "Italian" },
  { code: "LT", name: "Lithuania", calling: "370", language: "lt", locale: "lt-LT", languageName: "Lithuanian" },
  { code: "LV", name: "Latvia", calling: "371", language: "lv", locale: "lv-LV", languageName: "Latvian" },
  { code: "NL", name: "Netherlands", calling: "31", language: "nl", locale: "nl-NL", languageName: "Dutch" },
  { code: "NO", name: "Norway", calling: "47", language: "nb", locale: "nb-NO", languageName: "Norwegian" },
  { code: "PL", name: "Poland", calling: "48", language: "pl", locale: "pl-PL", languageName: "Polish" },
  { code: "PT", name: "Portugal", calling: "351", language: "pt", locale: "pt-PT", languageName: "Portuguese" },
  { code: "SE", name: "Sweden", calling: "46", language: "sv", locale: "sv-SE", languageName: "Swedish" },
  { code: "US", name: "United States", calling: "1", language: "en", locale: "en-US", languageName: "English" },
];

export const countryByCode = (code: string): Country | undefined => countries.find((c) => c.code === code.toUpperCase());
export const countryByName = (name: string): Country | undefined => countries.find((c) => c.name === name);

/** The made-up names of the country of a locale (nb-NO is Norway), as new lists. undefined when there are none. */
export function namesForLocale(locale: string): { first: string[]; last: string[] } | undefined {
  const c = countries.find((x) => x.locale === locale);
  const set = c && nameSets[c.code];
  return set ? { first: [...set.first], last: [...set.last] } : undefined;
}

/** The locales a user can prefer: the ones of the countries, with no repeats. */
export const locales: { code: string; label: string }[] = [
  ...new Map(countries.map((c) => [c.locale, { code: c.locale, label: `${c.languageName} (${c.locale})` }])).values(),
].sort((a, b) => a.label.localeCompare(b.label));

interface NameSet {
  first: string[];
  last: string[];
}

// names are separated by spaces, or by commas when a name has a space in it ("de Jong")
const split = (s: string) => (s.includes(",") ? s.split(",") : s.split(/\s+/)).map((n) => n.trim()).filter(Boolean);
const set = (first: string, last: string): NameSet => ({ first: split(first), last: split(last) });

/** Made-up names by country code. A country with no list here uses none: the wizard keeps the names that are there. */
export const nameSets: Record<string, NameSet> = {
  NO: set(
    "Ola Kari Nils Ingrid Håkon Sigrid Bjørn Astrid Øyvind Marte Lars Silje Åse Eirik Thea Jørgen Ida Magnus Solveig Tor Maja Even Live Sindre Hanne Åsmund Camilla Kristian Emilie Torbjørn Linn Henrik Aurora Mathias Nora Trygve Sunniva Vegard Mari",
    "Hansen Johansen Olsen Larsen Andersen Pedersen Nilsen Kristiansen Jensen Karlsen Johnsen Pettersen Eriksen Berg Haugen Hagen Johannessen Andreassen Jacobsen Dahl Jørgensen Halvorsen Henriksen Lund Sørensen Moen Gundersen Iversen Strand Solberg Svendsen Eide Knutsen Martinsen Paulsen Bakken Nygård Sæther Aas Myhre Løvik Ødegård",
  ),
  SE: set(
    "Erik Anna Lars Karin Johan Maria Anders Eva Mikael Sara Per Emma Karl Ida Olof Elin Nils Linnea Gustav Sofia Oskar Hanna Magnus Helena Fredrik Malin Jonas Kajsa Björn Åsa Henrik Frida Mattias Ingrid Viktor Elsa Axel Agnes",
    "Andersson Johansson Karlsson Nilsson Eriksson Larsson Olsson Persson Svensson Gustafsson Pettersson Jonsson Jansson Hansson Bengtsson Jönsson Lindberg Jakobsson Magnusson Olofsson Lindström Lindqvist Lindgren Axelsson Berg Bergström Lundberg Lind Lundgren Mattsson Berglund Fredriksson Sandberg Henriksson Forsberg Sjöberg",
  ),
  DK: set(
    "Mads Sofie Jens Mette Søren Anne Henrik Camilla Lars Louise Peter Katrine Thomas Maria Anders Line Rasmus Julie Mikkel Emma Kasper Ida Jesper Nanna Frederik Freja Morten Signe Christian Astrid Niels Cecilie Lasse Maja Emil Clara Oliver Laura",
    "Nielsen Jensen Hansen Pedersen Andersen Christensen Larsen Sørensen Rasmussen Jørgensen Petersen Madsen Kristensen Olsen Thomsen Christiansen Poulsen Johansen Møller Mortensen Knudsen Jakobsen Jacobsen Mikkelsen Olesen Frederiksen Laursen Henriksen Lund Schmidt Eriksen Holm Kristiansen Clausen Simonsen Svendsen",
  ),
  FI: set(
    "Juha Maria Mikko Johanna Matti Anna Timo Kaisa Antti Sanna Jari Laura Pekka Minna Hannu Heidi Janne Tiina Ville Elina Petri Noora Jukka Emilia Markus Satu Kari Riikka Tuomas Sari Olli Katja Eero Aino Lauri Helmi Aleksi Venla",
    "Korhonen Virtanen Mäkinen Nieminen Mäkelä Hämäläinen Laine Heikkinen Koskinen Järvinen Lehtonen Lehtinen Saarinen Salminen Heinonen Niemi Heikkilä Kinnunen Salonen Turunen Salo Laitinen Tuominen Rantanen Karjalainen Jokinen Mattila Savolainen Lahtinen Ahonen Kallio Leinonen Hakala",
  ),
  GB: set(
    "Oliver Olivia George Amelia Harry Isla Jack Ava Jacob Emily Noah Sophie Charlie Grace Thomas Mia Oscar Poppy William Ella James Lily Henry Freya Alfie Isabelle Leo Charlotte Archie Evie Joshua Ruby Samuel Hannah Daniel Megan Edward Holly",
    "Smith Jones Taylor Brown Williams Wilson Johnson Davies Robinson Wright Thompson Evans Walker White Roberts Green Hall Wood Jackson Clarke Patel Khan Lewis James Phillips Mason Mitchell Rose Davis Rogers Cook Cooper Ward Morris Moore Turner Hughes",
  ),
  US: set(
    "James Mary Robert Patricia John Jennifer Michael Linda William Elizabeth David Barbara Richard Susan Joseph Jessica Thomas Sarah Charles Karen Chris Emily Daniel Ashley Matthew Amanda Anthony Megan Mark Lauren Steven Rachel Andrew Nicole Kevin Brittany Brian Samantha",
    "Smith Johnson Williams Brown Jones Garcia Miller Davis Rodriguez Martinez Hernandez Lopez Gonzalez Wilson Anderson Thomas Taylor Moore Jackson Martin Lee Perez Thompson White Harris Sanchez Clark Ramirez Lewis Robinson Walker Young Allen King Wright Scott Torres Nguyen Hill Flores",
  ),
  DE: set(
    "Lukas Anna Jonas Lena Felix Laura Maximilian Julia Leon Sophie Paul Marie Elias Hannah Finn Lea Noah Mia Tim Katharina Jan Lisa Niklas Johanna Tobias Sarah Moritz Clara Florian Nina Stefan Petra Thomas Sabine Markus Andrea Matthias Claudia",
    "Müller Schmidt Schneider Fischer Weber Meyer Wagner Becker Schulz Hoffmann Schäfer Koch Bauer Richter Klein Wolf Schröder Neumann Schwarz Zimmermann Braun Krüger Hofmann Hartmann Lange Schmitt Werner Schmitz Krause Meier Lehmann Schmid Schulze Maier Köhler",
  ),
  NL: set(
    "Daan Emma Sem Julia Luuk Mila Bram Sophie Lars Tess Finn Anna Jesse Lotte Milan Eva Thijs Noor Sven Fleur Ruben Lisa Jasper Sanne Max Femke Tim Iris Joost Maud Pieter Marieke Hendrik Anouk Cornelis Saskia Willem Esther",
    "de Jong,Jansen,de Vries,van den Berg,van Dijk,Bakker,Janssen,Visser,Smit,Meijer,de Boer,Mulder,de Groot,Bos,Vos,Peters,Hendriks,van Leeuwen,Dekker,Brouwer,de Wit,Dijkstra,Smits,de Graaf,van der Meer,van der Linden,Kok,Jacobs,de Haan,Vermeulen,van den Heuvel,van der Veen,van den Broek",
  ),
  FR: set(
    "Lucas Emma Louis Jade Hugo Louise Gabriel Alice Arthur Chloé Jules Lina Adam Rose Raphaël Léa Paul Anna Nathan Manon Théo Camille Tom Inès Antoine Juliette Maxime Sarah Pierre Claire Nicolas Marie Julien Élodie Thomas Sophie Mathieu Céline",
    "Martin Bernard Thomas Petit Robert Richard Durand Dubois Moreau Laurent Simon Michel Lefebvre Leroy Roux David Bertrand Morel Fournier Girard Bonnet Dupont Lambert Fontaine Rousseau Vincent Müller Lefèvre Faure André Mercier Blanc Guerin Boyer Garnier",
  ),
  ES: set(
    "Hugo Lucía Mateo Sofía Martín Martina Lucas Paula Leo Julia Daniel Valeria Alejandro Daniela Manuel Alba Pablo Carla Álvaro Sara Adrián Noa David Carmen Javier Ana Diego Marta Carlos Laura Sergio Elena Miguel Isabel Jorge Cristina Rubén Irene",
    "García González Rodríguez Fernández López Martínez Sánchez Pérez Gómez Martín Jiménez Ruiz Hernández Díaz Moreno Álvarez Muñoz Romero Alonso Gutiérrez Navarro Torres Domínguez Vázquez Ramos Gil Ramírez Serrano Blanco Molina Morales Suárez Ortega Delgado Castro Ortiz Rubio",
  ),
  IT: set(
    "Leonardo Sofia Francesco Giulia Alessandro Aurora Lorenzo Alice Mattia Ginevra Andrea Emma Gabriele Giorgia Riccardo Greta Tommaso Beatrice Matteo Anna Davide Chiara Marco Sara Luca Martina Simone Elena Giovanni Francesca Paolo Valentina Stefano Silvia Roberto Laura Antonio Federica",
    "Rossi Russo Ferrari Esposito Bianchi Romano Colombo Ricci Marino Greco Bruno Gallo Conti De Luca Mancini Costa Giordano Rizzo Lombardi Moretti Barbieri Fontana Santoro Mariani Rinaldi Caruso Ferrara Galli Martini Leone Longo Gentile Martinelli Vitale",
  ),
  PL: set(
    "Jakub Zuzanna Szymon Julia Filip Maja Antoni Hanna Jan Lena Aleksander Alicja Franciszek Maria Mikołaj Oliwia Wojciech Amelia Michał Zofia Piotr Anna Paweł Katarzyna Tomasz Agnieszka Krzysztof Magdalena Marcin Joanna Łukasz Ewa Adam Monika Marek Barbara Andrzej Aleksandra",
    "Nowak Kowalski Wiśniewski Wójcik Kowalczyk Kamiński Lewandowski Zieliński Szymański Woźniak Dąbrowski Kozłowski Jankowski Mazur Kwiatkowski Krawczyk Piotrowski Grabowski Nowakowski Pawłowski Michalski Nowicki Adamczyk Dudek Zając Wieczorek Jabłoński Król Majewski Olszewski Jaworski Wróbel Malinowski Pawlak Witkowski Walczak",
  ),
};
