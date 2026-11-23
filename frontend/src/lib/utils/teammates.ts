type Teammate = { user_id: string; name?: string; email?: string };

export function teammateLabel({ user_id, name, email }: Teammate): string {
  if (name && email) return `${name} (${email})`;
  return name || email || user_id;
}
