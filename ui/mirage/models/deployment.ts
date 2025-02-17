import { Model, belongsTo } from 'ember-cli-mirage';
import { Deployment, Operation } from 'waypoint-pb';

const { PhysicalState } = Operation;
type StateName = keyof typeof PhysicalState;

export default Model.extend({
  application: belongsTo(),
  workspace: belongsTo(),
  build: belongsTo(),
  status: belongsTo({ inverse: 'owner' }),
  component: belongsTo({ inverse: 'owner' }),
  generation: belongsTo(),
  statusReport: belongsTo({ inverse: 'target' }),

  toProtobuf(): Deployment {
    let result = new Deployment();

    result.setApplication(this.application?.toProtobufRef());
    result.setArtifactId(this.artifactId);
    result.setComponent(this.component?.toProtobuf());
    result.setGeneration(this.generation?.toProtobuf());
    result.setHasEntrypointConfig(this.hasEntrypointConfig);
    result.setHasExecPlugin(this.hasExecPlugin);
    result.setHasLogsPlugin(this.hasLogsPlugin);
    result.setId(this.id);
    result.setJobId(this.jobId);
    result.setPreload(this.preloadProtobuf());
    result.setSequence(this.sequence);
    result.setState(PhysicalState[this.state as StateName]);
    result.setStatus(this.status?.toProtobuf());
    result.setTemplateData(this.templateData);
    result.setWorkspace(this.workspace?.toProtobufRef());

    for (let [key, value] of Object.entries<string>(this.labels ?? {})) {
      result.getLabelsMap().set(key, value);
    }

    return result;
  },

  preloadProtobuf(): Deployment.Preload {
    let result = new Deployment.Preload();

    // TODO: result.setArtifact
    result.setBuild(this.build?.toProtobuf());
    result.setDeployUrl(this.deployUrl);

    return result;
  },
});
