<?php

namespace FlowCatalyst\Generated\Endpoint;

class DeleteApplicationRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $roleName;
    /**
     * Deletes an SDK-sourced role. Cannot delete CODE or DATABASE sourced roles.
     * @param string $appCode
     * @param string $roleName
     */
    public function __construct(string $appCode, string $roleName)
    {
        $this->appCode = $appCode;
        $this->roleName = $roleName;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'DELETE';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{roleName}'], [$this->appCode, $this->roleName], '/api/applications/{appCode}/roles/{roleName}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationRoleNotFoundException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (204 === $status) {
            return null;
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\DeleteApplicationRoleBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\DeleteApplicationRoleNotFoundException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}