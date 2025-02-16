"use strict";

import { DateTime } from "./luxon.min.js";

const datetimeFormatter = () => {
    const datetime = document.querySelectorAll('time');
    datetime.forEach(function(element) {
        const dt = DateTime.fromISO(element.textContent);
        if (dt.isValid) {
            if (element.classList.contains('relative')) {
                element.textContent = dt.toRelative({ locale: document.documentElement.lang });
            } else {
                element.textContent = dt.toLocaleString(DateTime.DATE_FULL, { locale: document.documentElement.lang });
            }
        }
    });
}

document.addEventListener('DOMContentLoaded', datetimeFormatter());

const observer = new MutationObserver(datetimeFormatter);

// Start observing the target node for configured mutations
const node = document.getElementById("list");
observer.observe(node, { attributes: true, childList: false, subtree: true });
