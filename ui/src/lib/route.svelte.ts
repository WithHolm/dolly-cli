/** The place in the portal, kept in the hash: #/graph/api/groups. A hash needs no support from the server. */
class Route {
  segments = $state(read());

  constructor() {
    window.addEventListener("hashchange", () => (this.segments = read()));
  }

  /** The API: graph. */
  get api() {
    return this.segments[0] ?? "graph";
  }

  /** The page of the API: calls. */
  get page() {
    return this.segments[1] ?? "";
  }

  /** The address of a place. A part can have any character, for example "GET /v1.0/users". */
  href(...parts: string[]) {
    return "#/" + parts.filter(Boolean).map(encodeURIComponent).join("/");
  }

  go(...parts: string[]) {
    location.hash = this.href(...parts);
  }
}

function read(): string[] {
  return location.hash
    .replace(/^#\/?/, "")
    .split("/")
    .filter(Boolean)
    .map((part) => {
      try {
        return decodeURIComponent(part);
      } catch {
        return part; // a part that is not encoded right is kept as it is
      }
    });
}

export const route = new Route();
