/** Cache-busting query suffix from ?v= on this module's URL (import maps or versioned script src). */
export function versionQueryFromMeta() {
    const v = new URL(import.meta.url).searchParams.get('v')
    return v ? `?v=${v}` : ''
}

/** Dynamic import with the same ?v= as the current module. */
export function importVersioned(specifier) {
    return import(specifier + versionQueryFromMeta())
}
