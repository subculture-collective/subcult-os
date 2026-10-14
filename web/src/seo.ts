export const publicOrigin = (import.meta.env.VITE_PUBLIC_WEB_URL || 'https://os.subcult.tv').replace(/\/+$/, '');

export function eventMetadata(event: { publicSlug: string; title: string; publicDescription: string }) {
  document.querySelector('meta[name="robots"]')?.remove();
  document.title = `${event.title} | Subcult OS`;
  let canonical = document.querySelector<HTMLLinkElement>('link[rel="canonical"]');
  if (!canonical) { canonical = document.createElement('link'); canonical.rel = 'canonical'; document.head.append(canonical); }
  canonical.href = `${publicOrigin}/e/${encodeURIComponent(event.publicSlug)}`;
  const description = event.publicDescription.replace(/\s+/g, ' ').trim().slice(0, 180);
  const values = { description, 'og:title': document.title, 'og:description': description, 'og:url': canonical.href };
  for (const [key, value] of Object.entries(values)) {
    const attribute = key.startsWith('og:') ? 'property' : 'name';
    let meta = document.querySelector<HTMLMetaElement>(`meta[${attribute}="${key}"]`);
    if (!meta) { meta = document.createElement('meta'); meta.setAttribute(attribute, key); document.head.append(meta); }
    meta.content = value;
  }
}

export function unavailableEventMetadata() {
  document.title = 'Event unavailable | Subcult OS';
  document.querySelector('meta[property="og:url"]')?.remove();
  document.querySelector('link[rel="canonical"]')?.remove();
  let meta = document.querySelector<HTMLMetaElement>('meta[name="robots"]');
  if (!meta) { meta = document.createElement('meta'); meta.name = 'robots'; document.head.append(meta); }
  meta.content = 'noindex,nofollow';
}
