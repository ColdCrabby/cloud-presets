import { Routes } from '@angular/router';

export const routes: Routes = [
  {
    path: '',
    title: 'Browse presets',
    loadComponent: () => import('./pages/browse/browse').then((m) => m.Browse),
  },
];
