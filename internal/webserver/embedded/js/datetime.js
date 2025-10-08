"use strict";

// Extract version from current script URL for cache busting
const currentScript = document.currentScript?.src || import.meta.url;
const version = new URL(currentScript).searchParams.get('v') || '';
const versionParam = version ? `?v=${version}` : '';

import(`./luxon.min.js${versionParam}`).then(({ DateTime }) => {
    const datetimeFormatter = () => {
        const datetime = document.querySelectorAll('time.locale');
        datetime.forEach(function(element) {
            let dt = DateTime.fromISO(element.textContent);
            if (dt.isValid) {
                if (element.classList.contains('relative')) {
                    element.textContent = dt.toRelative({ locale: document.documentElement.lang });
                } else {
                    // This is a temporary fix to a bug in Luxon
                    // https://github.com/moment/luxon/issues/1687
                    if (dt.get('year') < 0) {
                        dt = dt.set({ year: dt.get('year') + 1 });
                    }
                    element.textContent = dt.toLocaleString(DateTime.DATE_FULL, { locale: document.documentElement.lang });
                }
            }
        });
    }

    document.addEventListener('DOMContentLoaded', datetimeFormatter());

    const observer = new MutationObserver(datetimeFormatter);

    // Start observing the target node for configured mutations
    const node = document.getElementsByTagName("body")[0];
    observer.observe(node, { attributes: true, childList: false, subtree: true });
});
