/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { createRoot } from 'react-dom/client';
import { initI18nInstance } from '@coze-arch/i18n/raw';
import { dynamicImportMdBoxStyle } from '@coze-arch/bot-md-box-adapter/style';
import { pullFeatureFlags, type FEATURE_FLAGS } from '@coze-arch/bot-flags';

import { App } from './app';
import './global.less';
import './index.less';

const runAutoLogin = async () => {
  try {
    const url = new URL(window.location.href);
    const token = url.searchParams.get('token');
    const name = url.searchParams.get('name');
    const type = url.searchParams.get('type');
    if (!name || !type) {
      return true;
    }
    if (token) {
      document.cookie = `token=${encodeURIComponent(token)}; path=/`;
    }
    console.log('token', token);
    const res = await fetch('/core-api/autologin', {
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ res_name: name, res_type: type }),
    });

    if (res.ok) {
      const payload = await res
        .clone()
        .json()
        .catch(() => null);
      const responseUrl =
        payload && typeof payload.url === 'string' ? payload.url.trim() : '';
      if (responseUrl) {
        window.location.replace(responseUrl);
        return false;
      }
      const redirect = url.searchParams.get('redirect');
      if (redirect && redirect.startsWith('/')) {
        window.location.replace(redirect);
        return false;
      }
    }
  } catch {
    // Ignore autologin failure and continue bootstrap.
  }

  return true;
};

const initFlags = () => {
  pullFeatureFlags({
    timeout: 1000 * 4,
    fetchFeatureGating: () => Promise.resolve({} as unknown as FEATURE_FLAGS),
  });
};

const main = () => {
  // Initialize the value of the function switch
  initFlags();
  // Initialize i18n
  initI18nInstance({
    lng: (localStorage.getItem('i18next') ?? (IS_OVERSEA ? 'en' : 'zh-CN')) as
      | 'en'
      | 'zh-CN',
  });
  // Import mdbox styles dynamically
  dynamicImportMdBoxStyle();

  const $root = document.getElementById('root');
  if (!$root) {
    throw new Error('root element not found');
  }
  const root = createRoot($root);

  root.render(<App />);
};

void runAutoLogin().then(shouldBootstrap => {
  if (shouldBootstrap) {
    main();
  }
});
