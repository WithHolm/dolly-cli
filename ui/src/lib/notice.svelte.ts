export interface Notice {
  id: number;
  kind: "info" | "error";
  text: string;
}

/** The messages that tell a person what happened after an action. */
class Notices {
  items = $state<Notice[]>([]);
  #next = 1;

  info(text: string) {
    const id = this.#add("info", text);
    setTimeout(() => this.dismiss(id), 3500);
  }

  /** An error stays longer, so it can be read, and goes when the person closes it or after 10 seconds. */
  error(text: string) {
    const id = this.#add("error", text);
    setTimeout(() => this.dismiss(id), 10_000);
  }

  dismiss(id: number) {
    this.items = this.items.filter((n) => n.id !== id);
  }

  #add(kind: Notice["kind"], text: string): number {
    const id = this.#next++;
    this.items = [...this.items, { id, kind, text }].slice(-4);
    return id;
  }
}

export const notices = new Notices();

/** succeeded runs an action that gives nothing back. It says if it worked; a failure becomes an error message. */
export async function succeeded(action: () => Promise<unknown>, done?: string): Promise<boolean> {
  return (await attempt(async () => (await action(), true), done)) ?? false;
}

/** attempt runs an action. A failure becomes an error message, and the result is undefined. */
export async function attempt<T>(action: () => Promise<T>, done?: string): Promise<T | undefined> {
  try {
    const value = await action();
    if (done) notices.info(done);
    return value;
  } catch (e) {
    notices.error(e instanceof Error ? e.message : String(e));
    return undefined;
  }
}
