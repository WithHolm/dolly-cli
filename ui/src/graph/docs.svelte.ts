import { graph } from "../lib/api";
import type { ApiDocs } from "./types";

/** What the server says about its operations. The API reference polls it, and a page that links to a call reads it. */
class ApiDocsStore {
  value = $state.raw<ApiDocs | null>(null);

  async load() {
    this.value = await graph.get<ApiDocs>("/api");
  }
}

export const apiDocs = new ApiDocsStore();
