import { renderToString } from 'react-dom/server';
import { DiscoverView } from './views/DiscoverView';
export { publicOrigin } from './seo';
export { discoveryDescription } from './modules/discovery/discoveryModel';

export function render() {
  return renderToString(<DiscoverView />);
}
