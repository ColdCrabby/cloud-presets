import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Button } from '@coldcrabby/ui';
import { Card } from '@cloud-presets/ui';
import { UploadService } from './upload.service';

/**
 * Upload panel on the dashboard: pick preset files (or a .zip laid out like the
 * presets repository), POST them, and jump to the claim page for the resulting
 * draft. The claim page is where the vendor reviews and opens the pull request.
 */
@Component({
  selector: 'ccc-upload-panel',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [Button, Card],
  templateUrl: './upload-panel.html',
  styleUrl: './upload-panel.scss',
})
export class UploadPanel {
  private readonly uploads = inject(UploadService);
  private readonly router = inject(Router);

  protected readonly kind = signal<'printer' | 'filament'>('printer');
  protected readonly fileNames = signal<string[]>([]);
  protected readonly error = signal<string | null>(null);
  protected readonly busy = this.uploads.busy;

  private files: File[] = [];

  protected onFiles(event: Event): void {
    const input = event.target as HTMLInputElement;
    this.files = input.files ? Array.from(input.files) : [];
    this.fileNames.set(this.files.map((f) => f.name));
    this.error.set(null);
  }

  protected setKind(value: 'printer' | 'filament'): void {
    this.kind.set(value);
  }

  protected get hasZip(): boolean {
    return this.files.some((f) => f.name.toLowerCase().endsWith('.zip'));
  }

  protected async submit(): Promise<void> {
    if (this.files.length === 0) {
      this.error.set('Choose at least one preset file or a .zip to upload.');
      return;
    }
    this.error.set(null);
    try {
      const res = await this.uploads.upload(this.files, this.hasZip ? undefined : this.kind());
      await this.router.navigate(['/claim', res.id]);
    } catch (e) {
      this.error.set(e instanceof Error ? e.message : String(e));
    }
  }
}
