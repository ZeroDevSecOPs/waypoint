import { ConfigVar, Project } from 'waypoint-pb';

import ApiService from 'waypoint/services/api';
import { BufferedChangeset } from 'ember-changeset/types';
import { Changeset } from 'ember-changeset';
import Component from '@glimmer/component';
import FlashMessagesService from 'waypoint/services/pds-flash-messages';
import { action } from '@ember/object';
import { inject as service } from '@ember/service';
import { tracked } from '@glimmer/tracking';

interface VariableArgs {
  variable: ConfigVar.AsObject;
  isEditing: boolean;
  isCreating: boolean;
  saveVariableSettings: (variable: BufferedChangeset, deleteVariable?: boolean) => Promise<Project.AsObject>;
  deleteVariable: (variable: ConfigVar.AsObject) => Promise<void>;
  cancelCreate: () => void;
}

export default class ProjectConfigVariablesListItemComponent extends Component<VariableArgs> {
  @service api!: ApiService;
  @service('pdsFlashMessages') flashMessages!: FlashMessagesService;

  initialVariable!: ConfigVar.AsObject;
  @tracked variable: ConfigVar.AsObject;
  @tracked changeset?: BufferedChangeset;
  @tracked isCreating: boolean;
  @tracked isEditing: boolean;

  constructor(owner: unknown, args: VariableArgs) {
    super(owner, args);
    let { variable, isEditing, isCreating } = args;
    this.variable = variable;
    this.isEditing = isEditing;
    this.isCreating = isCreating;

    if (this.isCreating || this.isEditing) {
      this.changeset = Changeset(this.variable);
    }
  }

  get isEditable(): boolean {
    // Temporarily making Dynamic vars uneditable
    return !this.variable.dynamic;
  }

  storeInitialVariable(): void {
    this.initialVariable = JSON.parse(JSON.stringify(this.variable));
  }

  @action
  async deleteVariable(variable: ConfigVar.AsObject): Promise<void> {
    await this.args.deleteVariable(variable);
  }

  @action
  editVariable(): void {
    this.isEditing = true;
    // this.storeInitialVariable();
    this.changeset = Changeset(this.variable);
  }

  @action
  async saveVariable(e: Event): Promise<void> {
    e.preventDefault();
    if (this.changeset.name === '' || this.changeset.pb_static === '') {
      this.flashMessages.error('Variable keys or values can not be empty');
      return;
    }
    await this.args.saveVariableSettings(this.changeset, false);
    this.changeset?.execute();
    this.isCreating = false;
    this.isEditing = false;
  }

  @action
  cancelCreate(): void {
    this.isCreating = false;
    this.isEditing = false;
    this.args.cancelCreate();
  }

  @action
  cancelEdit(): void {
    this.isCreating = false;
    this.isEditing = false;
    this.changeset?.unexecute();
    // this.variable = this.initialVariable;
  }
}
